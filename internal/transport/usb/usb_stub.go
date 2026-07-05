//go:build !usb

package usb

import (
	"io"
	"time"
)

// openLink is unavailable unless the binary is built with `-tags usb`.
func openLink(_ Options) (io.ReadWriteCloser, string, error) {
	return nil, "", ErrNotSupported
}

// List is unavailable unless the binary is built with `-tags usb`.
func List(_ time.Duration) ([]Candidate, error) {
	return nil, ErrNotSupported
}
