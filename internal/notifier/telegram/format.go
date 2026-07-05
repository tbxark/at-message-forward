package telegram

import (
	"fmt"
	"html"
	"strings"
	"time"
	"unicode/utf8"

	tgapp "github.com/go-sphere/telegram-bot/telegram"
	"github.com/go-telegram/bot/models"
	"github.com/tbxark/at-message-forward/internal/sms"
)

const smsBodyMax = 3000

const (
	truncationSuffix  = "\n[truncated]"
	fromPreviewMax    = 256
	watchdogReasonMax = 1200
	cmglItemMax       = 600
)

type commandResult struct {
	Command string
	Lines   []string
	Err     error
}

func formatSMSMessage(event sms.Event) *tgapp.Message {
	from := escapeAndTruncate(defaultText(event.From, "unknown"), fromPreviewMax)
	at := html.EscapeString(event.At.Format(time.RFC3339))
	body := escapeAndTruncate(defaultText(event.Text, "(empty)"), smsBodyMax)

	return &tgapp.Message{
		Text:      fmt.Sprintf("%s\n\n<b>Contact/Phone</b>: %s\n<b>Received</b>: %s", body, from, at),
		ParseMode: models.ParseModeHTML,
	}
}

func formatWatchdogAlert(reason string) *tgapp.Message {
	at := html.EscapeString(time.Now().Format(time.RFC3339))
	body := escapeAndTruncate(defaultText(reason, "serial watchdog reported an unknown error"), watchdogReasonMax)
	return &tgapp.Message{
		Text:      fmt.Sprintf("<b>Modem Watchdog Alert</b>\n\n%s\n\n<b>Time</b>: <code>%s</code>", body, at),
		ParseMode: models.ParseModeHTML,
		Button:    watchdogAlertKeyboard(),
	}
}

func formatCommandResult(results []commandResult) string {
	var b strings.Builder
	used := 0
	for _, result := range results {
		for _, line := range explainCommandResult(result) {
			if !appendHTMLChunk(&b, &used, line+"\n") {
				return strings.TrimSpace(b.String())
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func appendHTMLChunk(b *strings.Builder, used *int, chunk string) bool {
	chunkLen := utf8.RuneCountInString(chunk)
	suffixLen := utf8.RuneCountInString(truncationSuffix)
	if *used+chunkLen > maxText {
		if *used+suffixLen <= maxText {
			b.WriteString(truncationSuffix)
		}
		return false
	}
	if *used+chunkLen+suffixLen > maxText {
		b.WriteString(truncationSuffix)
		*used += suffixLen
		return false
	}
	b.WriteString(chunk)
	*used += chunkLen
	return true
}

func defaultText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func escapeAndTruncate(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	escaped := html.EscapeString(text)
	if utf8.RuneCountInString(escaped) <= limit {
		return escaped
	}
	suffixLen := utf8.RuneCountInString(truncationSuffix)
	var b strings.Builder
	used := 0
	for i, r := range text {
		escaped := html.EscapeString(string(r))
		width := utf8.RuneCountInString(escaped)
		if i+utf8.RuneLen(r) < len(text) && used+width+suffixLen > limit {
			b.WriteString(truncationSuffix)
			return b.String()
		}
		if used+width > limit {
			b.WriteString(truncationSuffix)
			return b.String()
		}
		b.WriteString(escaped)
		used += width
	}
	return b.String()
}
