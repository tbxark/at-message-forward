//go:build !usb

package usbserial

import (
	"io"
	"time"
)

// Open is unavailable unless the binary is built with `-tags usb`.
func Open(_ Options) (io.ReadWriteCloser, string, error) {
	return nil, "", ErrNotSupported
}

// List is unavailable unless the binary is built with `-tags usb`.
func List(_ time.Duration) ([]Candidate, error) {
	return nil, ErrNotSupported
}
