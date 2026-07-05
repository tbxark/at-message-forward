// Package transport abstracts how the modem's AT byte stream is reached. Every
// transport hands the forwarder an io.ReadWriteCloser (the "link"), regardless
// of whether it comes from a serial port, a raw USB interface, or a test fake.
//
// The concrete transports live in the serial/ and usb/ subpackages; New maps a
// config.Config onto the right one. New transports (TCP, a mock, ...) only need
// to implement Transport and get wired into New.
package transport

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/tbxark/at-message-forward/internal/config"
	"github.com/tbxark/at-message-forward/internal/transport/serial"
	"github.com/tbxark/at-message-forward/internal/transport/usb"
)

// Link is the modem AT byte stream a Transport hands to the session loop.
type Link = io.ReadWriteCloser

// Transport opens a connection to the modem's AT interface. Open returns a fresh
// Link plus a human-readable name for logging. Implementations must be
// re-openable so the forwarder can reconnect after a drop; for auto-detecting
// transports each Open re-runs discovery.
type Transport interface {
	Open(ctx context.Context) (Link, string, error)
}

// New builds the Transport selected by cfg.Transport ("usb" or, by default,
// "serial").
func New(cfg config.Config) (Transport, error) {
	if strings.EqualFold(cfg.Transport, config.TransportUSB) {
		vid, err := parseHexID(cfg.USBVendor)
		if err != nil {
			return nil, fmt.Errorf("usb_vendor: %w", err)
		}
		pid, err := parseHexID(cfg.USBProduct)
		if err != nil {
			return nil, fmt.Errorf("usb_product: %w", err)
		}
		return usb.New(usb.Options{
			Vendor:    vid,
			Product:   pid,
			Interface: cfg.USBInterface,
		}), nil
	}
	return serial.New(cfg.Port, cfg.Baud), nil
}

func parseHexID(value string) (uint16, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	value = strings.TrimPrefix(strings.ToLower(value), "0x")
	n, err := strconv.ParseUint(value, 16, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid hex id %q: %w", value, err)
	}
	return uint16(n), nil
}
