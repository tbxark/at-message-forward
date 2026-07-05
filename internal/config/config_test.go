package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg != Default() {
		t.Fatalf("Load() = %+v, want %+v", cfg, Default())
	}
}

func TestLoadJSONUsesDefaultForStringAndNumberZeroValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{
		"port": "",
		"baud": 0,
		"init_modem": false,
		"telegram_token": "",
		"telegram_chat": "12345"
	}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Baud != Default().Baud {
		t.Fatalf("Baud = %d, want default %d", cfg.Baud, Default().Baud)
	}
	if cfg.SIMReadyTimeoutSeconds != Default().SIMReadyTimeoutSeconds {
		t.Fatalf("SIMReadyTimeoutSeconds = %d, want default %d", cfg.SIMReadyTimeoutSeconds, Default().SIMReadyTimeoutSeconds)
	}
	if cfg.InitModem {
		t.Fatal("InitModem = true, want explicit false from JSON")
	}
	if cfg.TelegramChat != "12345" {
		t.Fatalf("TelegramChat = %q, want %q", cfg.TelegramChat, "12345")
	}
}

func TestLoadJSONUsesExplicitSIMReadyTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{
		"sim_ready_timeout_seconds": 180,
		"telegram_chat": "12345"
	}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.SIMReadyTimeoutSeconds != 180 {
		t.Fatalf("SIMReadyTimeoutSeconds = %d, want 180", cfg.SIMReadyTimeoutSeconds)
	}
}

func TestDefaultTransportIsSerial(t *testing.T) {
	if Default().Transport != TransportSerial {
		t.Fatalf("Default().Transport = %q, want %q", Default().Transport, TransportSerial)
	}
	if Default().USBInterface != -1 {
		t.Fatalf("Default().USBInterface = %d, want -1", Default().USBInterface)
	}
}

func TestLoadUSBTransport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{
		"transport": "usb",
		"usb_vendor": "2c7c",
		"usb_product": "0125",
		"usb_interface": 2,
		"telegram_chat": "12345"
	}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Transport != TransportUSB {
		t.Fatalf("Transport = %q, want %q", cfg.Transport, TransportUSB)
	}
	if cfg.USBVendor != "2c7c" || cfg.USBProduct != "0125" {
		t.Fatalf("USBVendor/Product = %q/%q, want 2c7c/0125", cfg.USBVendor, cfg.USBProduct)
	}
	if cfg.USBInterface != 2 {
		t.Fatalf("USBInterface = %d, want 2", cfg.USBInterface)
	}
}

func TestLoadOmittedUSBInterfaceStaysAuto(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"transport":"usb","telegram_chat":"1"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.USBInterface != -1 {
		t.Fatalf("USBInterface = %d, want -1 (auto)", cfg.USBInterface)
	}
}

func TestLoadInvalidJSONReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load returned nil error for invalid JSON")
	}
}

func TestLoadIgnoresUnknownConfigurePort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{
		"configure_port": false,
		"telegram_chat": "12345"
	}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.TelegramChat != "12345" {
		t.Fatalf("TelegramChat = %q, want %q", cfg.TelegramChat, "12345")
	}
}
