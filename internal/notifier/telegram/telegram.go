package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	tgapp "github.com/go-sphere/telegram-bot/telegram"
	telebot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/tbxark/at-message-forward/internal/config"
	"github.com/tbxark/at-message-forward/internal/notifier"
)

const (
	routeMenu = "menu"
	routeAct  = "act"
	maxText   = 3900
)

type Executor interface {
	ExecuteAT(context.Context, string) ([]string, error)
}

type Service struct {
	app      *tgapp.Bot
	chatID   int64
	executor Executor
}

type callbackData struct {
	ID string `json:"id"`
}

func New(cfg config.Config, executor Executor) (*Service, error) {
	if strings.TrimSpace(cfg.TelegramToken) == "" {
		return nil, errors.New("telegram_token is required")
	}
	chatID, err := ParseChatID(cfg.TelegramChat)
	if err != nil {
		return nil, err
	}
	if executor == nil {
		return nil, errors.New("telegram AT executor is required")
	}

	s := &Service{chatID: chatID, executor: executor}
	app, err := tgapp.NewApp(tgapp.Config{Token: cfg.TelegramToken}, tgapp.WithErrorHandler(s.handleError))
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}
	s.app = app
	s.bindRoutes()
	return s, nil
}

func ParseChatID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("telegram_chat is required")
	}
	chatID, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("telegram_chat must be an int64: %w", err)
	}
	return chatID, nil
}

func (s *Service) Start(ctx context.Context) error {
	return s.app.Start(ctx)
}

func (s *Service) Initialize(ctx context.Context) error {
	if err := s.registerCommands(ctx); err != nil {
		return err
	}
	return s.SendDefaultMenu(ctx)
}

func (s *Service) Close(ctx context.Context) error {
	return s.app.Close(ctx)
}

var _ notifier.Notifier = (*Service)(nil)

// Notify implements notifier.Notifier: it formats msg per its Kind and pushes
// it to the configured chat.
func (s *Service) Notify(ctx context.Context, msg notifier.Message) error {
	switch msg.Kind {
	case notifier.KindSMS:
		return s.sendMessage(ctx, s.chatID, formatSMSMessage(msg.SMS))
	case notifier.KindRaw:
		return s.pushText(ctx, "Modem raw: "+msg.Text)
	case notifier.KindWatchdog:
		return s.sendMessage(ctx, s.chatID, formatWatchdogAlert(msg.Text))
	default:
		return fmt.Errorf("telegram: unknown notify kind %d", msg.Kind)
	}
}

func (s *Service) SendDefaultMenu(ctx context.Context) error {
	return s.sendMessage(ctx, s.chatID, mainMenuMessage())
}

func (s *Service) pushText(ctx context.Context, text string) error {
	return s.sendMessage(ctx, s.chatID, &tgapp.Message{Text: text})
}

func (s *Service) sendMessage(ctx context.Context, chatID int64, msg *tgapp.Message) error {
	_, err := s.app.API().SendMessage(ctx, sendMessageParams(chatID, msg))
	return err
}

func (s *Service) registerCommands(ctx context.Context) error {
	_, err := s.app.API().SetMyCommands(ctx, &telebot.SetMyCommandsParams{Commands: defaultBotCommands()})
	return err
}

func (s *Service) bindRoutes() {
	s.app.BindCommand("start", s.showMainMenu)
	s.app.BindCommand("menu", s.showMainMenu)
	s.app.BindNoRoute(s.showMainMenu)
	s.app.BindCallback(routeMenu, s.handleMenu)
	s.app.BindCallback(routeAct, s.handleAction)
}

func (s *Service) showMainMenu(ctx context.Context, update *tgapp.Update) error {
	if !s.authorized(update) {
		return s.rejectUnauthorized(ctx, update)
	}
	return s.app.SendMessage(ctx, update, mainMenuMessage())
}

func (s *Service) handleMenu(ctx context.Context, update *tgapp.Update) error {
	if !s.authorized(update) {
		return s.rejectUnauthorized(ctx, update)
	}
	data, err := callbackPayload(update)
	if err != nil {
		return err
	}
	var msg *tgapp.Message
	switch data.ID {
	case "main":
		msg = mainMenuMessage()
	case "status":
		msg = statusMenuMessage()
	case "sms":
		msg = smsMenuMessage()
	case "device":
		msg = deviceMenuMessage()
	case "help":
		msg = helpMessage()
	case "reset_confirm":
		msg = resetConfirmMessage()
	default:
		msg = mainMenuMessage()
	}
	return s.app.SendMessage(ctx, update, msg)
}

func (s *Service) handleAction(ctx context.Context, update *tgapp.Update) error {
	if !s.authorized(update) {
		return s.rejectUnauthorized(ctx, update)
	}
	data, err := callbackPayload(update)
	if err != nil {
		return err
	}
	act, ok := actionForID(data.ID)
	if !ok {
		return s.app.SendMessage(ctx, update, &tgapp.Message{Text: "Unknown action", Button: backToMainKeyboard()})
	}
	results := s.runCommands(ctx, act.commands)
	return s.app.SendMessage(ctx, update, &tgapp.Message{
		Text:      formatCommandResult(results),
		ParseMode: models.ParseModeHTML,
		Button:    actionResultKeyboard(act.parent),
	})
}

func (s *Service) runCommands(ctx context.Context, commands []string) []commandResult {
	results := make([]commandResult, 0, len(commands))
	for _, cmd := range commands {
		lines, err := s.executor.ExecuteAT(ctx, cmd)
		results = append(results, commandResult{Command: cmd, Lines: lines, Err: err})
		if err != nil || ctx.Err() != nil {
			break
		}
	}
	return results
}

func (s *Service) authorized(update *tgapp.Update) bool {
	return AuthorizedChat(update, s.chatID)
}

func AuthorizedChat(update *tgapp.Update, chatID int64) bool {
	if update == nil {
		return false
	}
	if update.Message != nil {
		return update.Message.Chat.ID == chatID
	}
	if update.CallbackQuery != nil && update.CallbackQuery.Message.Message != nil {
		return update.CallbackQuery.Message.Message.Chat.ID == chatID
	}
	return false
}

func (s *Service) rejectUnauthorized(ctx context.Context, update *tgapp.Update) error {
	if update == nil {
		return nil
	}
	msg := &tgapp.Message{Text: "unauthorized chat"}
	if update.Message != nil || (update.CallbackQuery != nil && update.CallbackQuery.Message.Message != nil) {
		return s.app.SendMessage(ctx, update, msg)
	}
	return nil
}

func (s *Service) handleError(ctx context.Context, b *telebot.Bot, update *tgapp.Update, err error) {
	if err == nil {
		return
	}
	slog.Error("telegram handler failed", "err", err)
	if update == nil {
		return
	}
	tgapp.SendErrorMessage(ctx, b, update, err)
}

func callbackPayload(update *tgapp.Update) (callbackData, error) {
	if update == nil || update.CallbackQuery == nil {
		return callbackData{}, errors.New("missing callback query")
	}
	_, data, err := tgapp.UnmarshalData[callbackData](update.CallbackQuery.Data)
	if err != nil {
		return callbackData{}, err
	}
	if data == nil {
		return callbackData{}, errors.New("missing callback payload")
	}
	return *data, nil
}
