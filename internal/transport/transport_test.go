package transport

import (
	"testing"

	"github.com/tbxark/at-message-forward/internal/config"
	"github.com/tbxark/at-message-forward/internal/transport/serial"
	"github.com/tbxark/at-message-forward/internal/transport/usb"
)

func TestParseHexID(t *testing.T) {
	cases := []struct {
		in      string
		want    uint16
		wantErr bool
	}{
		{"", 0, false},
		{"2c7c", 0x2c7c, false},
		{"0x0125", 0x0125, false},
		{"FFFF", 0xffff, false},
		{"  2c7c  ", 0x2c7c, false},
		{"nothex", 0, true},
		{"10000", 0, true},
	}
	for _, c := range cases {
		got, err := parseHexID(c.in)
		if c.wantErr {
			if err == nil {
				t.Fatalf("parseHexID(%q) err = nil, want error", c.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseHexID(%q) err = %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("parseHexID(%q) = %#x, want %#x", c.in, got, c.want)
		}
	}
}

func TestNewSelectsUSBAndParsesOptions(t *testing.T) {
	tr, err := New(config.Config{Transport: "usb", USBVendor: "2c7c", USBInterface: 5})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	u, ok := tr.(*usb.Transport)
	if !ok {
		t.Fatalf("New returned %T, want *usb.Transport", tr)
	}
	if u.Options.Vendor != 0x2c7c {
		t.Fatalf("Vendor = %#x, want 0x2c7c", u.Options.Vendor)
	}
	if u.Options.Interface != 5 {
		t.Fatalf("Interface = %d, want 5", u.Options.Interface)
	}
}

func TestNewRejectsBadUSBHex(t *testing.T) {
	if _, err := New(config.Config{Transport: "usb", USBProduct: "nothex"}); err == nil {
		t.Fatal("New with bad usb_product = nil error, want error")
	}
}

func TestNewDefaultsToSerial(t *testing.T) {
	tr, err := New(config.Config{Port: "/dev/ttyUSB0", Baud: 9600})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	s, ok := tr.(*serial.Transport)
	if !ok {
		t.Fatalf("New returned %T, want *serial.Transport", tr)
	}
	if s.Port != "/dev/ttyUSB0" || s.Baud != 9600 {
		t.Fatalf("serial transport = %+v, want port /dev/ttyUSB0 baud 9600", s)
	}
}
