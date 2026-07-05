package modem

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	modemat "github.com/warthog618/modem/at"
)

type fakeCommander struct {
	responses []fakeResponse
	commands  []string
}

type fakeResponse struct {
	lines []string
	err   error
}

func (f *fakeCommander) Command(cmd string, _ ...modemat.CommandOption) ([]string, error) {
	f.commands = append(f.commands, cmd)
	if len(f.responses) == 0 {
		return nil, errors.New("unexpected command")
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response.lines, response.err
}

func TestInitRetriesCPINUntilReady(t *testing.T) {
	modem := &fakeCommander{responses: []fakeResponse{
		{},
		{},
		{err: errors.New("CME Error: 13")},
		{lines: []string{"+CPIN: READY"}},
		{lines: []string{"+CSQ: 20,99"}},
		{},
		{},
	}}

	if err := initModem(modem, time.Second, 0); err != nil {
		t.Fatalf("initModem returned error: %v", err)
	}

	want := []string{"", "E0", "+CPIN?", "+CPIN?", "+CSQ", "+CMGF=1", "+CNMI=2,2,0,0,0"}
	if !reflect.DeepEqual(modem.commands, want) {
		t.Fatalf("commands = %v, want %v", modem.commands, want)
	}
}

func TestWaitForCPINReadyTimesOut(t *testing.T) {
	modem := &fakeCommander{responses: []fakeResponse{
		{err: errors.New("CME Error: 13")},
	}}

	err := waitForCPINReady(modem, 0, 0)
	if err == nil {
		t.Fatal("waitForCPINReady returned nil error")
	}
	if !strings.Contains(err.Error(), "did not report READY") {
		t.Fatalf("error = %q, want timeout message", err.Error())
	}
}
