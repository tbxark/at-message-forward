package forwarder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/tbxark/at-message-forward/internal/config"
	"github.com/tbxark/at-message-forward/internal/modem"
	"github.com/tbxark/at-message-forward/internal/sms"
	"github.com/tbxark/at-message-forward/internal/telegrambot"
	"github.com/tbxark/at-message-forward/internal/transport"
	modemat "github.com/warthog618/modem/at"
)

const (
	// Empty suffix makes modem.Command send a bare AT probe.
	serialWatchdogCommand    = ""
	serialWatchdogInterval   = time.Minute
	serialWatchdogThreshold  = 3
	serialWatchdogAlertLimit = 10 * time.Second
	reconnectInitialBackoff  = 2 * time.Second
	reconnectMaxBackoff      = 30 * time.Second
	telegramImportantBuffer  = 64
	telegramRawBuffer        = 16
)

var (
	errSerialNotConnected  = errors.New("serial not connected")
	errSerialSessionClosed = errors.New("serial session closed")
	newTransport           = transport.New
)

type telegramClient interface {
	SendSMS(context.Context, sms.Event) error
	SendRaw(context.Context, string) error
	SendWatchdogAlert(context.Context, string) error
}

type reconnectableExecutor struct {
	commandMu sync.Mutex
	stateMu   sync.RWMutex
	modem     *modemat.AT
}

func (e *reconnectableExecutor) Set(modem *modemat.AT) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	e.modem = modem
}

func (e *reconnectableExecutor) Clear(modem *modemat.AT) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	if e.modem == modem {
		e.modem = nil
	}
}

func (e *reconnectableExecutor) ExecuteAT(ctx context.Context, cmd string) ([]string, error) {
	e.commandMu.Lock()
	defer e.commandMu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	e.stateMu.RLock()
	atModem := e.modem
	e.stateMu.RUnlock()
	if atModem == nil {
		return nil, errSerialNotConnected
	}
	return modem.RunATCommand(atModem, cmd)
}

type telegramSendKind int

const (
	telegramSendSMS telegramSendKind = iota
	telegramSendRaw
	telegramSendWatchdog
)

type telegramSendItem struct {
	kind   telegramSendKind
	event  sms.Event
	line   string
	reason string
}

type telegramSender struct {
	client    telegramClient
	important chan telegramSendItem
	raw       chan telegramSendItem
}

func newTelegramSender(client telegramClient) *telegramSender {
	return &telegramSender{
		client:    client,
		important: make(chan telegramSendItem, telegramImportantBuffer),
		raw:       make(chan telegramSendItem, telegramRawBuffer),
	}
}

func (s *telegramSender) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-s.important:
			s.send(ctx, item)
		case item := <-s.raw:
			s.send(ctx, item)
		}
	}
}

func (s *telegramSender) EnqueueSMS(ctx context.Context, event sms.Event) {
	s.enqueueImportant(ctx, telegramSendItem{kind: telegramSendSMS, event: event})
}

func (s *telegramSender) EnqueueWatchdog(ctx context.Context, reason string) {
	s.enqueueImportant(ctx, telegramSendItem{kind: telegramSendWatchdog, reason: reason})
}

func (s *telegramSender) EnqueueRaw(line string) {
	select {
	case s.raw <- telegramSendItem{kind: telegramSendRaw, line: line}:
	default:
		slog.Warn("telegram raw queue full; dropping raw line", "line", line)
	}
}

func (s *telegramSender) enqueueImportant(ctx context.Context, item telegramSendItem) {
	select {
	case <-ctx.Done():
	case s.important <- item:
	}
}

func (s *telegramSender) send(ctx context.Context, item telegramSendItem) {
	var err error
	switch item.kind {
	case telegramSendSMS:
		err = s.client.SendSMS(ctx, item.event)
	case telegramSendRaw:
		err = s.client.SendRaw(ctx, item.line)
	case telegramSendWatchdog:
		alertCtx, cancel := context.WithTimeout(ctx, serialWatchdogAlertLimit)
		err = s.client.SendWatchdogAlert(alertCtx, item.reason)
		cancel()
	}
	if err != nil && ctx.Err() == nil {
		slog.Error("telegram send failed", "kind", item.kind, "err", err)
	}
}

type reconnectLoopOptions struct {
	runSession     func(context.Context) error
	wait           func(context.Context, time.Duration) error
	initialBackoff time.Duration
	maxBackoff     time.Duration
}

func Run(ctx context.Context, cfg config.Config) error {
	if cfg.TelegramToken == "" {
		return fmt.Errorf("telegram_token is required")
	}
	if _, err := telegrambot.ParseChatID(cfg.TelegramChat); err != nil {
		return err
	}

	executor := &reconnectableExecutor{}
	telegram, err := telegrambot.New(cfg, executor)
	if err != nil {
		return err
	}
	if err := telegram.Initialize(ctx); err != nil {
		_ = telegram.Close(context.Background())
		return fmt.Errorf("initialize telegram bot failed: %w", err)
	}
	pollCtx, cancelPoll := context.WithCancel(ctx)
	defer func() {
		cancelPoll()
		_ = telegram.Close(context.Background())
	}()
	go func() {
		if err := telegram.Start(pollCtx); err != nil && pollCtx.Err() == nil {
			slog.Error("telegram polling failed", "err", err)
		}
	}()
	slog.Info("telegram forwarding and polling control enabled")
	sender := newTelegramSender(telegram)
	go sender.Run(ctx)

	watchdogCtx, cancelWatchdog := context.WithCancel(ctx)
	defer cancelWatchdog()
	go runSerialWatchdog(watchdogCtx, executor, sender)

	tr, err := newTransport(cfg)
	if err != nil {
		return err
	}
	return runReconnectLoop(ctx, cfg, tr, executor, sender)
}

func runReconnectLoop(ctx context.Context, cfg config.Config, tr transport.Transport, executor *reconnectableExecutor, sender *telegramSender) error {
	return runReconnectLoopWithOptions(ctx, reconnectLoopOptions{
		runSession: func(ctx context.Context) error {
			return runModemSession(ctx, cfg, tr, executor, sender)
		},
		wait:           waitBackoff,
		initialBackoff: reconnectInitialBackoff,
		maxBackoff:     reconnectMaxBackoff,
	})
}

func runReconnectLoopWithOptions(ctx context.Context, opts reconnectLoopOptions) error {
	initialBackoff := opts.initialBackoff
	if initialBackoff <= 0 {
		initialBackoff = reconnectInitialBackoff
	}
	backoff := initialBackoff
	maxBackoff := opts.maxBackoff
	if maxBackoff <= 0 {
		maxBackoff = reconnectMaxBackoff
	}
	runSession := opts.runSession
	wait := opts.wait
	if wait == nil {
		wait = waitBackoff
	}

	for {
		if ctx.Err() != nil {
			slog.Info("stopped")
			return nil
		}
		err := runSession(ctx)
		if ctx.Err() != nil {
			slog.Info("stopped")
			return nil
		}
		if err != nil {
			slog.Warn("serial session ended; reconnecting", "err", err, "backoff", backoff)
		} else {
			slog.Warn("serial session ended; reconnecting", "backoff", backoff)
		}
		waitDelay := backoff
		if errors.Is(err, errSerialSessionClosed) {
			waitDelay = initialBackoff
		}
		if err := wait(ctx, waitDelay); err != nil {
			return nil
		}
		if errors.Is(err, errSerialSessionClosed) {
			backoff = initialBackoff
		} else {
			backoff = nextBackoff(waitDelay, maxBackoff)
		}
	}
}

// runModemSession opens the transport (re-running discovery each call) and runs
// one modem session over the returned link.
func runModemSession(ctx context.Context, cfg config.Config, tr transport.Transport, executor *reconnectableExecutor, sender *telegramSender) error {
	link, name, err := tr.Open(ctx)
	if err != nil {
		return fmt.Errorf("open modem failed: %w", err)
	}
	return runOpenedModemSession(ctx, cfg, name, link, executor, sender)
}

func runOpenedModemSession(ctx context.Context, cfg config.Config, portName string, port io.ReadWriteCloser, executor *reconnectableExecutor, sender *telegramSender) error {
	var atModem *modemat.AT
	defer func() {
		if atModem != nil {
			executor.Clear(atModem)
		}
		if err := port.Close(); err != nil {
			slog.Warn("serial port close failed", "err", err)
		}
	}()

	events := make(chan sms.Event, 8)
	rawLines := make(chan string, 32)
	atModem = modem.NewAT(port, rawLines, events)

	if cfg.InitModem {
		if err := modem.Init(atModem, time.Duration(cfg.SIMReadyTimeoutSeconds)*time.Second); err != nil {
			return fmt.Errorf("initialize modem failed: %w", err)
		}
	}
	executor.Set(atModem)

	slog.Info("listening for SMS; press Ctrl+C to stop")
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-atModem.Closed():
			slog.Info("serial connection closed")
			sender.EnqueueWatchdog(ctx, fmt.Sprintf("serial connection closed on %s", portName))
			return errSerialSessionClosed
		case line := <-rawLines:
			slog.Info("raw line received", "at", time.Now().Format(time.RFC3339), "line", line)
			if cfg.TelegramRaw {
				sender.EnqueueRaw(line)
			}
		case event := <-events:
			slog.Info("sms received", "at", event.At.Format(time.RFC3339), "from", event.From, "text", event.Text)
			sender.EnqueueSMS(ctx, event)
		}
	}
}

func waitBackoff(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}

func runSerialWatchdog(ctx context.Context, executor telegrambot.Executor, sender *telegramSender) {
	ticker := time.NewTicker(serialWatchdogInterval)
	defer ticker.Stop()

	failures := 0
	alerted := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := probeSerial(ctx, executor)
			if err == nil {
				if failures > 0 {
					slog.Info("serial watchdog recovered", "failures", failures)
				}
				failures = 0
				alerted = false
				continue
			}

			failures++
			slog.Warn("serial watchdog probe failed", "failures", failures, "threshold", serialWatchdogThreshold, "err", err)
			if failures < serialWatchdogThreshold || alerted {
				continue
			}

			alerted = true
			reason := fmt.Sprintf("serial watchdog probe failed %d consecutive times: %v", failures, err)
			sender.EnqueueWatchdog(ctx, reason)
		}
	}
}

func probeSerial(ctx context.Context, executor telegrambot.Executor) error {
	_, err := executor.ExecuteAT(ctx, serialWatchdogCommand)
	return err
}
