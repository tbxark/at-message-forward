package modem

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/tbxark/at-message-forward/internal/sms"
	modemat "github.com/warthog618/modem/at"
)

const (
	cpinCommand      = "+CPIN?"
	cpinReadyLine    = "+CPIN: READY"
	cpinReadyTimeout = 120 * time.Second
	cpinRetryDelay   = 2 * time.Second
)

type atCommander interface {
	Command(string, ...modemat.CommandOption) ([]string, error)
}

func NewAT(port io.ReadWriter, rawLines chan<- string, events chan<- sms.Event) *modemat.AT {
	return modemat.New(port,
		modemat.WithTimeout(5*time.Second),
		modemat.WithIndication("+CMT:", func(lines []string) {
			emitRawLines(rawLines, lines)
			event, err := sms.ParseCMTIndication(lines)
			if err != nil {
				slog.Error("parse +CMT failed", "err", err)
				return
			}
			emitSMSEvent(events, event)
		}, modemat.WithTrailingLine),
		modemat.WithIndication("+CMTI:", func(lines []string) {
			emitRawLines(rawLines, lines)
		}),
		modemat.WithIndication("+CDS:", func(lines []string) {
			emitRawLines(rawLines, lines)
		}, modemat.WithTrailingLine),
	)
}

// Init runs the generic 3GPP AT initialization sequence supported by most
// AT-capable cellular modems (SIMCom, Quectel, EigenComm/Air780E, etc.):
// echo off, wait for SIM ready, then enable direct SMS push in text mode.
func Init(modem atCommander, simReadyTimeout time.Duration) error {
	if simReadyTimeout <= 0 {
		simReadyTimeout = cpinReadyTimeout
	}
	return initModem(modem, simReadyTimeout, cpinRetryDelay)
}

func initModem(modem atCommander, cpinTimeout, cpinDelay time.Duration) error {
	commands := []string{
		"",
		"E0",
	}
	for _, cmd := range commands {
		if _, err := RunATCommand(modem, cmd); err != nil {
			return err
		}
	}

	if err := waitForCPINReady(modem, cpinTimeout, cpinDelay); err != nil {
		return err
	}

	commands = []string{
		"+CSQ",
		"+CMGF=1",
		"+CNMI=2,2,0,0,0",
	}
	for _, cmd := range commands {
		if _, err := RunATCommand(modem, cmd); err != nil {
			return err
		}
	}
	return nil
}

func waitForCPINReady(modem atCommander, timeout, retryDelay time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		lines, err := RunATCommand(modem, cpinCommand)
		if err == nil {
			if hasLine(lines, cpinReadyLine) {
				return nil
			}
			return fmt.Errorf("AT%s returned %q, want %s", cpinCommand, lines, cpinReadyLine)
		}
		lastErr = err
		if timeout <= 0 || time.Now().Add(retryDelay).After(deadline) {
			break
		}
		slog.Warn("SIM not ready; retrying", "err", err, "retry_in", retryDelay)
		time.Sleep(retryDelay)
	}
	return fmt.Errorf("AT%s did not report READY within %s: %w", cpinCommand, timeout, lastErr)
}

func hasLine(lines []string, want string) bool {
	for _, line := range lines {
		if strings.EqualFold(strings.TrimSpace(line), want) {
			return true
		}
	}
	return false
}

func RunATCommand(modem atCommander, cmd string) ([]string, error) {
	display := "AT" + cmd
	slog.Info("at command tx", "command", display)
	info, err := modem.Command(cmd)
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", display, err)
	}
	for _, line := range info {
		slog.Info("at command rx", "line", line)
	}
	return info, nil
}

func emitRawLines(rawLines chan<- string, lines []string) {
	for _, line := range lines {
		select {
		case rawLines <- line:
		default:
			slog.Warn("raw line dropped", "line", line)
		}
	}
}

func emitSMSEvent(events chan<- sms.Event, event sms.Event) {
	select {
	case events <- event:
	default:
		slog.Warn("sms event dropped", "from", event.From)
	}
}
