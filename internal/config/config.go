package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const DefaultPath = "config.json"

type Config struct {
	// Transport selects how the modem is reached: "serial" (default, opens a
	// /dev/tty* or /dev/cu.* serial port) or "usb" (talks to the modem's AT
	// interface directly over USB bulk endpoints via libusb; needed on macOS
	// for modems whose AT port is a vendor-specific USB class with no serial
	// driver, e.g. Quectel EC25). The "usb" transport requires a binary built
	// with `-tags usb` and CGO_ENABLED=1.
	Transport              string `json:"transport"`
	Port                   string `json:"port"`
	Baud                   int    `json:"baud"`
	InitModem              bool   `json:"init_modem"`
	SIMReadyTimeoutSeconds int    `json:"sim_ready_timeout_seconds"`
	// USBVendor/USBProduct optionally pin the USB transport to a specific
	// device (hex, e.g. "2c7c" / "0125"). Empty scans all connected devices.
	USBVendor  string `json:"usb_vendor"`
	USBProduct string `json:"usb_product"`
	// USBInterface forces a specific USB interface number for the AT port.
	// -1 (default) auto-probes vendor-specific interfaces for one that answers AT.
	USBInterface  int    `json:"usb_interface"`
	TelegramRaw   bool   `json:"telegram_raw"`
	TelegramToken string `json:"telegram_token"`
	TelegramChat  string `json:"telegram_chat"`
}

const (
	TransportSerial = "serial"
	TransportUSB    = "usb"
)

func Default() Config {
	return Config{
		Transport:              TransportSerial,
		Port:                   "",
		Baud:                   115200,
		InitModem:              true,
		SIMReadyTimeoutSeconds: 120,
		USBInterface:           -1,
		TelegramToken:          "",
		TelegramChat:           "",
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = DefaultPath
	}

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("open config %q: %w", path, err)
	}
	defer func() {
		_ = f.Close()
	}()

	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}
	cfg.applyDefaults(Default())
	return cfg, nil
}

func (c *Config) applyDefaults(defaults Config) {
	if c.Transport == "" {
		c.Transport = defaults.Transport
	}
	if c.Port == "" {
		c.Port = defaults.Port
	}
	if c.Baud == 0 {
		c.Baud = defaults.Baud
	}
	if c.SIMReadyTimeoutSeconds == 0 {
		c.SIMReadyTimeoutSeconds = defaults.SIMReadyTimeoutSeconds
	}
	if c.TelegramToken == "" {
		c.TelegramToken = defaults.TelegramToken
	}
	if c.TelegramChat == "" {
		c.TelegramChat = defaults.TelegramChat
	}
}
