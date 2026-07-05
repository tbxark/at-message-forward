package telegram

import (
	tgapp "github.com/go-sphere/telegram-bot/telegram"
	telebot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func mainMenuMessage() *tgapp.Message {
	return &tgapp.Message{
		Text: "AT Modem Console\nChoose an action.",
		Button: oneColumnKeyboard(
			menuButton("Status", "status"),
			menuButton("SMS History", "sms"),
			menuButton("Device Control", "device"),
			menuButton("Help", "help"),
		),
	}
}

func defaultBotCommands() []models.BotCommand {
	return []models.BotCommand{
		{Command: "start", Description: "Open the default control menu"},
		{Command: "menu", Description: "Open the control menu"},
	}
}

func sendMessageParams(chatID int64, msg *tgapp.Message) *telebot.SendMessageParams {
	if msg == nil {
		msg = &tgapp.Message{}
	}
	params := &telebot.SendMessageParams{
		ChatID:    chatID,
		Text:      truncateText(msg.Text),
		ParseMode: msg.ParseMode,
	}
	if len(msg.Button) > 0 {
		params.ReplyMarkup = &models.InlineKeyboardMarkup{InlineKeyboard: msg.Button}
	}
	return params
}

func statusMenuMessage() *tgapp.Message {
	return &tgapp.Message{
		Text: "Status",
		Button: oneColumnKeyboard(
			actionButton("Status Summary", "status_summary"),
			actionButton("Signal Quality", "signal"),
			actionButton("Network Registration", "registration"),
			actionButton("Operator", "operator"),
			actionButton("SIM Status", "sim"),
			actionButton("Module Info", "module"),
			menuButton("Back", "main"),
		),
	}
}

func smsMenuMessage() *tgapp.Message {
	return &tgapp.Message{
		Text: "SMS History",
		Button: oneColumnKeyboard(
			actionButton("Unread SMS", "sms_unread"),
			actionButton("All SMS", "sms_all"),
			actionButton("SMS Storage", "sms_storage"),
			actionButton("Store SMS to SIM", "enable_sms_storage"),
			menuButton("Back", "main"),
		),
	}
}

func deviceMenuMessage() *tgapp.Message {
	return &tgapp.Message{
		Text: "Device Control",
		Button: oneColumnKeyboard(
			actionButton("Current Function Mode", "function_mode"),
			actionButton("Re-enable SMS Push", "enable_sms_push"),
			menuButton("Restart Module", "reset_confirm"),
			menuButton("Back", "main"),
		),
	}
}

func resetConfirmMessage() *tgapp.Message {
	return &tgapp.Message{
		Text: "Restart the modem module? The serial connection may drop during reboot.",
		Button: oneColumnKeyboard(
			actionButton("Confirm Restart", "reset"),
			menuButton("Cancel / Back", "device"),
		),
	}
}

func watchdogAlertKeyboard() [][]models.InlineKeyboardButton {
	return oneColumnKeyboard(
		actionButton("Restart Module", "reset"),
		menuButton("Device Control", "device"),
	)
}

func helpMessage() *tgapp.Message {
	return &tgapp.Message{
		Text: "Send /menu to open the control menu. New SMS messages are pushed to the authorized chat automatically. When telegram_raw=true, raw serial lines are pushed too.",
		Button: oneColumnKeyboard(
			menuButton("Back", "main"),
		),
	}
}

func backToMainKeyboard() [][]models.InlineKeyboardButton {
	return oneColumnKeyboard(menuButton("Back to Main Menu", "main"))
}

func actionResultKeyboard(parent string) [][]models.InlineKeyboardButton {
	if parent == "" || parent == "main" {
		return backToMainKeyboard()
	}
	return oneColumnKeyboard(
		menuButton("Back", parent),
		menuButton("Main Menu", "main"),
	)
}

func oneColumnKeyboard(buttons ...models.InlineKeyboardButton) [][]models.InlineKeyboardButton {
	rows := make([][]models.InlineKeyboardButton, 0, len(buttons))
	for _, button := range buttons {
		rows = append(rows, []models.InlineKeyboardButton{button})
	}
	return rows
}

func menuButton(text, id string) models.InlineKeyboardButton {
	return tgapp.NewButton(text, routeMenu, callbackData{ID: id})
}

func actionButton(text, id string) models.InlineKeyboardButton {
	return tgapp.NewButton(text, routeAct, callbackData{ID: id})
}

func truncateText(text string) string {
	return truncatePlainText(text, maxText)
}

func truncatePlainText(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	cut := limit - len([]rune(truncationSuffix))
	if cut < 0 {
		cut = 0
	}
	return string(runes[:cut]) + truncationSuffix
}
