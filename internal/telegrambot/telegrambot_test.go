package telegrambot

import (
	"errors"
	"strings"
	"testing"
	"time"

	tgapp "github.com/go-sphere/telegram-bot/telegram"
	"github.com/go-telegram/bot/models"
	"github.com/tbxark/at-message-forward/internal/sms"
)

func TestParseChatID(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    int64
		wantErr bool
	}{
		{name: "valid", value: "123456789", want: 123456789},
		{name: "valid negative", value: "-100123456789", want: -100123456789},
		{name: "trim spaces", value: " 42 ", want: 42},
		{name: "empty", value: "", wantErr: true},
		{name: "not int", value: "chat", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseChatID(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("chatID = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAuthorizedChat(t *testing.T) {
	messageUpdate := &tgapp.Update{Message: &models.Message{Chat: models.Chat{ID: 100}}}
	if !AuthorizedChat(messageUpdate, 100) {
		t.Fatal("message chat should be authorized")
	}
	if AuthorizedChat(messageUpdate, 200) {
		t.Fatal("different message chat should not be authorized")
	}

	callbackUpdate := &tgapp.Update{CallbackQuery: &models.CallbackQuery{Message: models.MaybeInaccessibleMessage{Message: &models.Message{Chat: models.Chat{ID: 100}}}}}
	if !AuthorizedChat(callbackUpdate, 100) {
		t.Fatal("callback chat should be authorized")
	}
	if AuthorizedChat(&tgapp.Update{}, 100) {
		t.Fatal("empty update should not be authorized")
	}
}

func TestActionForID(t *testing.T) {
	act, ok := actionForID("sms_all")
	if !ok {
		t.Fatal("sms_all action not found")
	}
	if act.parent != "sms" {
		t.Fatalf("parent = %q, want sms", act.parent)
	}
	want := []string{"+CMGF=1", "+CMGL=\"ALL\""}
	if len(act.commands) != len(want) {
		t.Fatalf("commands len = %d, want %d", len(act.commands), len(want))
	}
	for i := range want {
		if act.commands[i] != want[i] {
			t.Fatalf("commands[%d] = %q, want %q", i, act.commands[i], want[i])
		}
	}
	if _, ok := actionForID("missing"); ok {
		t.Fatal("missing action should not exist")
	}
}

func mustUnmarshalCallbackData(t *testing.T, value string) callbackData {
	t.Helper()
	_, data, err := tgapp.UnmarshalData[callbackData](value)
	if err != nil {
		t.Fatalf("unmarshal callback: %v", err)
	}
	if data == nil {
		t.Fatal("unmarshal callback returned nil data")
	}
	return *data
}

func TestActionResultKeyboard(t *testing.T) {
	keyboard := actionResultKeyboard("sms")
	if len(keyboard) != 2 {
		t.Fatalf("keyboard rows = %d, want 2", len(keyboard))
	}
	for i, row := range keyboard {
		if len(row) != 1 {
			t.Fatalf("keyboard row %d has %d buttons, want 1", i, len(row))
		}
	}
	backData := mustUnmarshalCallbackData(t, keyboard[0][0].CallbackData)
	if backData.ID != "sms" {
		t.Fatalf("back id = %q, want sms", backData.ID)
	}
	mainData := mustUnmarshalCallbackData(t, keyboard[1][0].CallbackData)
	if mainData.ID != "main" {
		t.Fatalf("main id = %q, want main", mainData.ID)
	}
}

func TestAllActionsHaveParentMenus(t *testing.T) {
	for _, id := range []string{"status_summary", "signal", "registration", "operator", "sim", "module", "sms_unread", "sms_all", "sms_storage", "function_mode", "enable_sms_push", "reset"} {
		act, ok := actionForID(id)
		if !ok {
			t.Fatalf("action %q not found", id)
		}
		if act.parent == "" {
			t.Fatalf("action %q has empty parent", id)
		}
	}
}

func TestMainMenuButtons(t *testing.T) {
	msg := mainMenuMessage()
	if len(msg.Button) != 4 {
		t.Fatalf("button rows = %d, want 4", len(msg.Button))
	}
	for i, row := range msg.Button {
		if len(row) != 1 {
			t.Fatalf("button row %d has %d buttons, want 1", i, len(row))
		}
	}
	if !strings.HasPrefix(msg.Button[0][0].CallbackData, routeMenu+":") {
		t.Fatalf("status callback = %q", msg.Button[0][0].CallbackData)
	}
	data := mustUnmarshalCallbackData(t, msg.Button[0][0].CallbackData)
	if data.ID != "status" {
		t.Fatalf("callback id = %q, want status", data.ID)
	}
}

func TestSubmenusUseSingleColumn(t *testing.T) {
	menus := []*tgapp.Message{statusMenuMessage(), smsMenuMessage(), deviceMenuMessage(), resetConfirmMessage(), helpMessage()}
	for _, msg := range menus {
		for i, row := range msg.Button {
			if len(row) != 1 {
				t.Fatalf("%q row %d has %d buttons, want 1", msg.Text, i, len(row))
			}
		}
	}
}

func TestWatchdogAlertIncludesRestartButton(t *testing.T) {
	msg := formatWatchdogAlert("serial closed <bad>")
	if msg.ParseMode != models.ParseModeHTML {
		t.Fatalf("ParseMode = %q, want HTML", msg.ParseMode)
	}
	for _, want := range []string{"<b>Modem Watchdog Alert</b>", "serial closed &lt;bad&gt;"} {
		if !strings.Contains(msg.Text, want) {
			t.Fatalf("watchdog alert missing %q: %s", want, msg.Text)
		}
	}
	if strings.Contains(msg.Text, "<pre>") {
		t.Fatalf("watchdog alert should not use code blocks: %s", msg.Text)
	}
	reasonAt := strings.Index(msg.Text, "serial closed &lt;bad&gt;")
	timeAt := strings.Index(msg.Text, "<b>Time</b>")
	if reasonAt < 0 || timeAt < 0 || reasonAt > timeAt {
		t.Fatalf("watchdog reason should appear before metadata: %s", msg.Text)
	}
	if len(msg.Button) == 0 || len(msg.Button[0]) != 1 {
		t.Fatalf("watchdog alert missing restart button: %#v", msg.Button)
	}
	if !strings.HasPrefix(msg.Button[0][0].CallbackData, routeAct+":") {
		t.Fatalf("restart callback = %q", msg.Button[0][0].CallbackData)
	}
	data := mustUnmarshalCallbackData(t, msg.Button[0][0].CallbackData)
	if data.ID != "reset" {
		t.Fatalf("restart callback id = %q, want reset", data.ID)
	}
}

func TestDefaultBotCommands(t *testing.T) {
	commands := defaultBotCommands()
	if len(commands) != 2 {
		t.Fatalf("commands len = %d, want 2", len(commands))
	}
	if commands[0].Command != "start" {
		t.Fatalf("commands[0] = %q, want start", commands[0].Command)
	}
	if commands[1].Command != "menu" {
		t.Fatalf("commands[1] = %q, want menu", commands[1].Command)
	}
	for _, command := range commands {
		if strings.HasPrefix(command.Command, "/") {
			t.Fatalf("command %q should not include leading slash", command.Command)
		}
		if command.Description == "" {
			t.Fatalf("command %q has empty description", command.Command)
		}
	}
}

func TestSendMessageParamsIncludesDefaultMenu(t *testing.T) {
	msg := mainMenuMessage()
	params := sendMessageParams(12345, msg)
	if params.ChatID != int64(12345) {
		t.Fatalf("ChatID = %v, want 12345", params.ChatID)
	}
	if params.Text != msg.Text {
		t.Fatalf("Text = %q, want %q", params.Text, msg.Text)
	}
	markup, ok := params.ReplyMarkup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("ReplyMarkup = %T, want *models.InlineKeyboardMarkup", params.ReplyMarkup)
	}
	if len(markup.InlineKeyboard) != len(msg.Button) {
		t.Fatalf("keyboard rows = %d, want %d", len(markup.InlineKeyboard), len(msg.Button))
	}
}

func TestFormatCommandResultAndTruncation(t *testing.T) {
	text := formatCommandResult([]commandResult{
		{Command: "+CSQ", Lines: []string{"+CSQ: 20,99", "OK"}},
		{Command: "+COPS?", Err: errors.New("timeout")},
	})
	for _, want := range []string{"Signal strength", "Execution failed", "timeout"} {
		if !strings.Contains(text, want) {
			t.Fatalf("formatted text missing %q: %s", want, text)
		}
	}
	for _, unwanted := range []string{"<b>Test</b>", "AT+CSQ", "+CSQ: 20,99", "Raw response"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("formatted text includes unwanted %q: %s", unwanted, text)
		}
	}

	long := truncateText(strings.Repeat("x", maxText+10))
	if len([]rune(long)) > maxText {
		t.Fatalf("truncated length = %d, max %d", len([]rune(long)), maxText)
	}
	if !strings.HasSuffix(long, "[truncated]") {
		t.Fatalf("missing truncation suffix: %q", long[len(long)-20:])
	}
}

func TestEscapeAndTruncateKeepsHTMLEntitiesValid(t *testing.T) {
	text := escapeAndTruncate("<&>", 12)
	if text != "\n[truncated]" {
		t.Fatalf("text = %q, want truncation suffix only", text)
	}

	text = escapeAndTruncate("abc<&>", 15)
	if text != "abc\n[truncated]" {
		t.Fatalf("text = %q, want safe truncated prefix", text)
	}

	text = escapeAndTruncate("abc", 3)
	if text != "abc" {
		t.Fatalf("text = %q, want unmodified escaped text", text)
	}
}

func TestFormatSMSMessageUsesHTML(t *testing.T) {
	msg := formatSMSMessage(sms.Event{From: "+123<456>", Text: "hello <world>&\nline2", At: time.Date(2026, 6, 19, 10, 20, 30, 0, time.UTC)})
	if msg.ParseMode != models.ParseModeHTML {
		t.Fatalf("ParseMode = %q, want HTML", msg.ParseMode)
	}
	for _, want := range []string{"hello &lt;world&gt;&amp;", "<b>Contact/Phone</b>: +123&lt;456&gt;", "<b>Received</b>: 2026-06-19T10:20:30Z"} {
		if !strings.Contains(msg.Text, want) {
			t.Fatalf("SMS text missing %q: %s", want, msg.Text)
		}
	}
	for _, unwanted := range []string{"<b>New SMS</b>", "<pre>", "<code>"} {
		if strings.Contains(msg.Text, unwanted) {
			t.Fatalf("SMS text should not contain %q: %s", unwanted, msg.Text)
		}
	}
	if !strings.HasPrefix(msg.Text, "hello &lt;world&gt;&amp;\nline2") {
		t.Fatalf("SMS body should be at the top: %s", msg.Text)
	}
	bodyAt := strings.Index(msg.Text, "hello &lt;world&gt;&amp;")
	senderAt := strings.Index(msg.Text, "<b>Contact/Phone</b>")
	receivedAt := strings.Index(msg.Text, "<b>Received</b>")
	if bodyAt < 0 || senderAt < 0 || receivedAt < 0 || bodyAt > senderAt || senderAt > receivedAt {
		t.Fatalf("SMS body should appear before sender and received metadata: %s", msg.Text)
	}
}

func TestExplainCMGLMessageLayoutAvoidsCodeBlocks(t *testing.T) {
	text := strings.Join(explainCMGL([]string{`+CMGL: 1,"REC READ","+123<456>",,"26/06/19,18:00:00+32"`, "hello <world>&", "OK"}), "\n")
	for _, want := range []string{"Read 1 SMS message(s).", "<b>SMS #1</b> REC READ", "hello &lt;world&gt;&amp;", "<b>Sender</b>: <code>+123&lt;456&gt;</code>", "<b>Received</b>: <code>26/06/19,18:00:00+32</code>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("SMS history text missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "<pre>") {
		t.Fatalf("SMS history should not use code blocks: %s", text)
	}
	bodyAt := strings.Index(text, "hello &lt;world&gt;&amp;")
	senderAt := strings.Index(text, "<b>Sender</b>")
	receivedAt := strings.Index(text, "<b>Received</b>")
	if bodyAt < 0 || senderAt < 0 || receivedAt < 0 || bodyAt > senderAt || senderAt > receivedAt {
		t.Fatalf("SMS history body should appear before sender and received metadata: %s", text)
	}
}

func TestExplainCommandResults(t *testing.T) {
	tests := []struct {
		name   string
		result commandResult
		want   string
	}{
		{name: "registration", result: commandResult{Command: "+CEREG?", Lines: []string{"+CEREG: 0,1", "OK"}}, want: "registered on home network"},
		{name: "operator", result: commandResult{Command: "+COPS?", Lines: []string{"+COPS: 0,0,\"CHINA MOBILE\",7", "OK"}}, want: "LTE/E-UTRAN"},
		{name: "storage", result: commandResult{Command: "+CPMS?", Lines: []string{"+CPMS: \"SM\",1,50,\"SM\",1,50,\"SM\",1,50", "OK"}}, want: "used 1 / total 50"},
		{name: "sms list", result: commandResult{Command: "+CMGL=\"ALL\"", Lines: []string{"+CMGL: 1,\"REC READ\",\"+123\",,\"26/06/19,18:00:00+32\"", "hello", "OK"}}, want: "Read 1 SMS message(s)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := strings.Join(explainCommandResult(tt.result), "\n")
			if !strings.Contains(text, tt.want) {
				t.Fatalf("explanation missing %q: %s", tt.want, text)
			}
		})
	}
}
