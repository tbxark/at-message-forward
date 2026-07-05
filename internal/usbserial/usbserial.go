// Package usbserial provides an io.ReadWriteCloser transport that talks to a
// cellular modem's AT command interface directly over USB bulk endpoints.
//
// This is needed on hosts (notably macOS) where the modem's AT port is exposed
// as a vendor-specific USB interface (class 0xFF) that no kernel serial driver
// binds, so no /dev/tty*/dev/cu.* node exists. Linux usually creates
// /dev/ttyUSB* for such modems via the `option` driver, in which case the plain
// serial transport is simpler and this package is unnecessary.
package usbserial

import (
	"errors"
	"time"
)

// Options selects and configures the USB modem transport.
type Options struct {
	// Vendor and Product optionally pin discovery to a specific USB device.
	// Zero means "any device that exposes a vendor-specific bulk interface".
	Vendor  uint16
	Product uint16
	// Interface forces a specific USB interface number for the AT port.
	// A negative value auto-probes candidate interfaces for one that answers AT.
	Interface int
	// ProbeTimeout bounds each per-interface AT probe.
	ProbeTimeout time.Duration
}

// Candidate describes a discovered USB modem for the ports listing.
type Candidate struct {
	VID          uint16
	PID          uint16
	Interface    int
	Manufacturer string
	ProductName  string
	ProbeOK      bool
	ProbeError   string
}

// DefaultProbeTimeout is the per-interface AT probe timeout when unset.
const DefaultProbeTimeout = 1500 * time.Millisecond

// ErrNotSupported is returned by the stub build (without `-tags usb`).
var ErrNotSupported = errors.New("usb transport not built in; rebuild with `-tags usb` and CGO_ENABLED=1 (requires libusb)")
