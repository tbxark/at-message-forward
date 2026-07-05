// Package notifier abstracts where forwarded modem messages are pushed to. It
// is the dual of the transport package: transport abstracts how the AT byte
// stream is read in, notifier abstracts how the resulting messages are pushed
// out (Telegram today; email, webhook, Bark, ... tomorrow).
//
// A Notifier only covers the push-out direction. Channel-specific extras — the
// Telegram bot's menus, long polling, and AT command control — deliberately
// live in the concrete implementation (notifier/telegram), not in this
// interface. Adding a channel means implementing Notify and nothing else.
package notifier

import (
	"context"

	"github.com/tbxark/at-message-forward/internal/sms"
)

// Kind classifies a pushed message so a Notifier can format it appropriately.
type Kind int

const (
	// KindSMS is a received SMS; the SMS field is populated.
	KindSMS Kind = iota
	// KindRaw is an unparsed modem line; the Text field holds the line.
	KindRaw
	// KindWatchdog is a connectivity alert; the Text field holds the reason.
	KindWatchdog
)

// Message is a single item to push out. Exactly one payload field is meaningful
// per Kind: SMS for KindSMS, Text for KindRaw and KindWatchdog.
type Message struct {
	Kind Kind
	SMS  sms.Event
	Text string
}

// SMSMessage builds a KindSMS message.
func SMSMessage(event sms.Event) Message {
	return Message{Kind: KindSMS, SMS: event}
}

// RawMessage builds a KindRaw message from an unparsed modem line.
func RawMessage(line string) Message {
	return Message{Kind: KindRaw, Text: line}
}

// WatchdogMessage builds a KindWatchdog alert from a reason string.
func WatchdogMessage(reason string) Message {
	return Message{Kind: KindWatchdog, Text: reason}
}

// Notifier delivers a Message to an outbound channel. Implementations format the
// Message according to its Kind. Notify should honor ctx for cancellation and
// timeouts.
type Notifier interface {
	Notify(ctx context.Context, msg Message) error
}
