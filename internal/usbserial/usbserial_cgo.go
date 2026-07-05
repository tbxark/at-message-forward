//go:build usb

package usbserial

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/gousb"
)

// stageBufferSize is the USB bulk read buffer. It must be a multiple of the
// endpoint max packet size (512 for high-speed bulk) to avoid overflow.
const stageBufferSize = 16 * 1024

// transport implements io.ReadWriteCloser over a claimed USB bulk in/out pair.
type transport struct {
	ctx    *gousb.Context
	dev    *gousb.Device
	cfg    *gousb.Config
	intf   *gousb.Interface
	in     *gousb.InEndpoint
	out    *gousb.OutEndpoint
	rctx   context.Context
	cancel context.CancelFunc

	stage     []byte
	pending   []byte
	closeOnce sync.Once
}

func (t *transport) Read(p []byte) (int, error) {
	for len(t.pending) == 0 {
		if t.rctx.Err() != nil {
			return 0, io.EOF
		}
		n, err := t.in.ReadContext(t.rctx, t.stage)
		if n > 0 {
			t.pending = t.stage[:n]
			break
		}
		if err != nil {
			if t.rctx.Err() != nil {
				return 0, io.EOF
			}
			return 0, err
		}
	}
	n := copy(p, t.pending)
	t.pending = t.pending[n:]
	return n, nil
}

func (t *transport) Write(p []byte) (int, error) {
	return t.out.WriteContext(t.rctx, p)
}

func (t *transport) Close() error {
	t.closeOnce.Do(func() {
		t.cancel()
		if t.intf != nil {
			t.intf.Close()
		}
		if t.cfg != nil {
			_ = t.cfg.Close()
		}
		if t.dev != nil {
			_ = t.dev.Close()
		}
		if t.ctx != nil {
			_ = t.ctx.Close()
		}
	})
	return nil
}

// Open finds the modem, selects its AT interface, and returns a transport plus
// a human-readable description of what was opened.
func Open(opts Options) (io.ReadWriteCloser, string, error) {
	timeout := opts.ProbeTimeout
	if timeout <= 0 {
		timeout = DefaultProbeTimeout
	}

	usbctx := gousb.NewContext()
	ok := false
	defer func() {
		if !ok {
			_ = usbctx.Close()
		}
	}()

	dev, err := findDevice(usbctx, opts.Vendor, opts.Product)
	if err != nil {
		return nil, "", err
	}
	defer func() {
		if !ok {
			_ = dev.Close()
		}
	}()

	// Do NOT auto-detach kernel drivers: on macOS the modem's CDC-ECM network
	// interface is bound by the kernel and detaching it fails; we only need the
	// unclaimed vendor-specific AT interface.
	cfg, err := dev.Config(1)
	if err != nil {
		return nil, "", fmt.Errorf("claim usb config: %w", err)
	}
	defer func() {
		if !ok {
			_ = cfg.Close()
		}
	}()

	mfr, _ := dev.Manufacturer()
	prod, _ := dev.Product()

	var lastErr error
	for _, ifNum := range candidateInterfaces(dev.Desc, opts.Interface) {
		intf, in, out, err := claimInterface(cfg, ifNum)
		if err != nil {
			lastErr = err
			continue
		}
		if opts.Interface >= 0 || probeAT(out, in, timeout) {
			rctx, cancel := context.WithCancel(context.Background())
			t := &transport{
				ctx: usbctx, dev: dev, cfg: cfg, intf: intf, in: in, out: out,
				rctx: rctx, cancel: cancel, stage: make([]byte, stageBufferSize),
			}
			ok = true
			name := fmt.Sprintf("usb %s:%s if%d (%s %s)",
				dev.Desc.Vendor, dev.Desc.Product, ifNum,
				strings.TrimSpace(mfr), strings.TrimSpace(prod))
			return t, strings.TrimSpace(name), nil
		}
		intf.Close()
	}
	if lastErr == nil {
		lastErr = errors.New("no AT-capable USB interface found")
	}
	return nil, "", lastErr
}

// List enumerates connected modems and reports which interface answers AT.
func List(probeTimeout time.Duration) ([]Candidate, error) {
	if probeTimeout <= 0 {
		probeTimeout = DefaultProbeTimeout
	}
	usbctx := gousb.NewContext()
	defer usbctx.Close()

	devs, err := usbctx.OpenDevices(func(d *gousb.DeviceDesc) bool {
		return hasVendorBulkInterface(d)
	})
	defer func() {
		for _, d := range devs {
			_ = d.Close()
		}
	}()
	if err != nil && len(devs) == 0 {
		return nil, err
	}

	out := make([]Candidate, 0, len(devs))
	for _, dev := range devs {
		c := Candidate{VID: uint16(dev.Desc.Vendor), PID: uint16(dev.Desc.Product), Interface: -1}
		c.Manufacturer, _ = dev.Manufacturer()
		c.ProductName, _ = dev.Product()

		cfg, err := dev.Config(1)
		if err != nil {
			c.ProbeError = err.Error()
			out = append(out, c)
			continue
		}
		for _, ifNum := range candidateInterfaces(dev.Desc, -1) {
			intf, in, o, err := claimInterface(cfg, ifNum)
			if err != nil {
				continue
			}
			okAT := probeAT(o, in, probeTimeout)
			intf.Close()
			if okAT {
				c.Interface = ifNum
				c.ProbeOK = true
				break
			}
		}
		if !c.ProbeOK {
			c.ProbeError = "no interface answered AT"
		}
		_ = cfg.Close()
		out = append(out, c)
	}
	return out, nil
}

func findDevice(ctx *gousb.Context, vid, pid uint16) (*gousb.Device, error) {
	devs, err := ctx.OpenDevices(func(d *gousb.DeviceDesc) bool {
		if vid != 0 && uint16(d.Vendor) != vid {
			return false
		}
		if pid != 0 && uint16(d.Product) != pid {
			return false
		}
		return hasVendorBulkInterface(d)
	})
	if len(devs) == 0 {
		if err != nil {
			return nil, fmt.Errorf("enumerate usb devices: %w", err)
		}
		return nil, errors.New("no matching USB modem found (need a vendor-specific bulk interface)")
	}
	// Keep the first match, close the rest.
	for _, d := range devs[1:] {
		_ = d.Close()
	}
	return devs[0], nil
}

// candidateInterfaces returns the interface numbers worth probing for an AT
// port: vendor-specific (class 0xFF) interfaces that have both a bulk IN and a
// bulk OUT endpoint. When forced >= 0, only that interface is returned.
func candidateInterfaces(desc *gousb.DeviceDesc, forced int) []int {
	if forced >= 0 {
		return []int{forced}
	}
	seen := make(map[int]bool)
	var nums []int
	for _, cfg := range desc.Configs {
		for _, intf := range cfg.Interfaces {
			for _, alt := range intf.AltSettings {
				if uint8(alt.Class) != 0xff {
					continue
				}
				hasIn, hasOut := false, false
				for _, ep := range alt.Endpoints {
					if ep.TransferType != gousb.TransferTypeBulk {
						continue
					}
					if ep.Direction == gousb.EndpointDirectionIn {
						hasIn = true
					} else {
						hasOut = true
					}
				}
				if hasIn && hasOut && !seen[intf.Number] {
					seen[intf.Number] = true
					nums = append(nums, intf.Number)
				}
			}
		}
	}
	sort.Ints(nums)
	return nums
}

func hasVendorBulkInterface(d *gousb.DeviceDesc) bool {
	return len(candidateInterfaces(d, -1)) > 0
}

func claimInterface(cfg *gousb.Config, ifNum int) (*gousb.Interface, *gousb.InEndpoint, *gousb.OutEndpoint, error) {
	intf, err := cfg.Interface(ifNum, 0)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("claim if%d: %w", ifNum, err)
	}
	inNum, outNum := -1, -1
	for _, ep := range intf.Setting.Endpoints {
		if ep.TransferType != gousb.TransferTypeBulk {
			continue
		}
		if ep.Direction == gousb.EndpointDirectionIn {
			inNum = ep.Number
		} else {
			outNum = ep.Number
		}
	}
	if inNum < 0 || outNum < 0 {
		intf.Close()
		return nil, nil, nil, fmt.Errorf("if%d has no bulk in/out pair", ifNum)
	}
	in, err := intf.InEndpoint(inNum)
	if err != nil {
		intf.Close()
		return nil, nil, nil, fmt.Errorf("if%d in endpoint: %w", ifNum, err)
	}
	out, err := intf.OutEndpoint(outNum)
	if err != nil {
		intf.Close()
		return nil, nil, nil, fmt.Errorf("if%d out endpoint: %w", ifNum, err)
	}
	return intf, in, out, nil
}

// probeAT sends a bare AT and reports whether the interface replies with OK
// within the timeout. A GPS/NMEA port streams data without OK and fails.
func probeAT(out *gousb.OutEndpoint, in *gousb.InEndpoint, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if _, err := out.WriteContext(ctx, []byte("AT\r")); err != nil {
		return false
	}
	var acc []byte
	buf := make([]byte, stageBufferSize)
	for ctx.Err() == nil {
		n, err := in.ReadContext(ctx, buf)
		if n > 0 {
			acc = append(acc, buf[:n]...)
			if bytes.Contains(acc, []byte("OK")) {
				return true
			}
		}
		if err != nil {
			return false
		}
	}
	return false
}
