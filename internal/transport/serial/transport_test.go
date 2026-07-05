package serial

import (
	"context"
	"errors"
	"testing"

	bugst "go.bug.st/serial"
)

func TestTransportOpenAutoDetectsEachCall(t *testing.T) {
	originalAutoDetect := autoDetectPort
	originalOpen := openPort
	defer func() {
		autoDetectPort = originalAutoDetect
		openPort = originalOpen
	}()

	autoDetectCalls := 0
	autoDetectPort = func(int) (string, error) {
		autoDetectCalls++
		return "/dev/ttyUSB-test", nil
	}
	openPort = func(string, int) (bugst.Port, error) {
		return nil, errors.New("open failed")
	}

	tr := New("", 115200)
	for i := 0; i < 2; i++ {
		if _, _, err := tr.Open(context.Background()); err == nil {
			t.Fatal("Open err = nil, want error")
		}
	}
	if autoDetectCalls != 2 {
		t.Fatalf("autoDetectCalls = %d, want 2", autoDetectCalls)
	}
}

func TestTransportOpenSkipsAutoDetectWithExplicitPort(t *testing.T) {
	originalAutoDetect := autoDetectPort
	originalOpen := openPort
	defer func() {
		autoDetectPort = originalAutoDetect
		openPort = originalOpen
	}()

	autoDetectPort = func(int) (string, error) {
		t.Fatal("auto-detect should not run when Port is set")
		return "", nil
	}
	var gotPort string
	openPort = func(port string, _ int) (bugst.Port, error) {
		gotPort = port
		return nil, errors.New("open failed")
	}

	tr := New("/dev/ttyUSB9", 115200)
	if _, _, err := tr.Open(context.Background()); err == nil {
		t.Fatal("Open err = nil, want error")
	}
	if gotPort != "/dev/ttyUSB9" {
		t.Fatalf("openPort port = %q, want /dev/ttyUSB9", gotPort)
	}
}
