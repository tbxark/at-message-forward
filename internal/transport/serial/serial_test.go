package serial

import (
	"errors"
	"testing"
	"time"
)

func TestRankCandidates(t *testing.T) {
	candidates := RankCandidates([]Candidate{
		{Port: "/dev/ttyACM2", Source: "glob", Score: 10},
		{Port: "/dev/ttyACM0", Source: "by-id:usb-EigenComm_Compo-if03", Score: 190},
		{Port: "/dev/ttyACM1", Source: "by-id:usb-EigenComm_Compo-if05", Score: 170},
		{Port: "/dev/ttyACM0", Source: "glob", Score: 10},
	})
	if len(candidates) != 3 {
		t.Fatalf("len = %d", len(candidates))
	}
	if candidates[0].Port != "/dev/ttyACM0" {
		t.Fatalf("first port = %q", candidates[0].Port)
	}
	if candidates[0].Source != "by-id:usb-EigenComm_Compo-if03" {
		t.Fatalf("first source = %q", candidates[0].Source)
	}
}

func TestAutoDetectWithBaudPrefersProbeOK(t *testing.T) {
	originalCandidateProvider := candidateProvider
	originalProbeSerialPort := probeSerialPort
	defer func() {
		candidateProvider = originalCandidateProvider
		probeSerialPort = originalProbeSerialPort
	}()

	candidateProvider = func() []Candidate {
		return []Candidate{
			{Port: "/dev/ttyUSB0", Source: "by-id:usb-EigenComm-if03", Score: 180},
			{Port: "/dev/ttyUSB1", Source: "by-id:usb-EigenComm-if02", Score: 150},
		}
	}
	probeSerialPort = func(port string, baud int, timeout time.Duration) (bool, error) {
		if baud != 9600 {
			t.Fatalf("baud = %d, want 9600", baud)
		}
		if timeout != DefaultProbeTimeout {
			t.Fatalf("timeout = %s, want %s", timeout, DefaultProbeTimeout)
		}
		if port == "/dev/ttyUSB1" {
			return true, nil
		}
		return false, errors.New("no AT response")
	}

	port, err := AutoDetectWithBaud(9600)
	if err != nil {
		t.Fatalf("AutoDetectWithBaud returned error: %v", err)
	}
	if port != "/dev/ttyUSB1" {
		t.Fatalf("port = %q, want /dev/ttyUSB1", port)
	}
}

func TestAutoDetectWithBaudFallsBackToRankedCandidate(t *testing.T) {
	originalCandidateProvider := candidateProvider
	originalProbeSerialPort := probeSerialPort
	defer func() {
		candidateProvider = originalCandidateProvider
		probeSerialPort = originalProbeSerialPort
	}()

	candidateProvider = func() []Candidate {
		return []Candidate{
			{Port: "/dev/ttyUSB0", Source: "by-id:usb-EigenComm-if03", Score: 180},
			{Port: "/dev/ttyUSB1", Source: "by-id:usb-EigenComm-if02", Score: 150},
		}
	}
	probeSerialPort = func(string, int, time.Duration) (bool, error) {
		return false, errors.New("no AT response")
	}

	port, err := AutoDetectWithBaud(115200)
	if err != nil {
		t.Fatalf("AutoDetectWithBaud returned error: %v", err)
	}
	if port != "/dev/ttyUSB0" {
		t.Fatalf("port = %q, want /dev/ttyUSB0", port)
	}
}

func TestProbeCandidatesMovesATOKFirst(t *testing.T) {
	originalProbeSerialPort := probeSerialPort
	defer func() {
		probeSerialPort = originalProbeSerialPort
	}()

	probeSerialPort = func(port string, baud int, timeout time.Duration) (bool, error) {
		if port == "/dev/ttyUSB1" {
			return true, nil
		}
		return false, errors.New("no AT response")
	}

	candidates := ProbeCandidates([]Candidate{
		{Port: "/dev/ttyUSB0", Source: "ranked-first", Score: 180},
		{Port: "/dev/ttyUSB1", Source: "ranked-second", Score: 150},
	}, 115200, time.Second)
	if len(candidates) != 2 {
		t.Fatalf("len = %d, want 2", len(candidates))
	}
	if candidates[0].Port != "/dev/ttyUSB1" || !candidates[0].ProbeOK {
		t.Fatalf("first candidate = %+v, want probed OK /dev/ttyUSB1", candidates[0])
	}
	if !candidates[1].ProbeAttempted || candidates[1].ProbeError == "" {
		t.Fatalf("second candidate probe metadata = %+v, want failed probe metadata", candidates[1])
	}
}

func TestResponseHasATLine(t *testing.T) {
	if !responseHasATLine("\r\nAT\r\r\nOK\r\n", "OK") {
		t.Fatal("expected OK line")
	}
	if responseHasATLine("\r\nBROKEN\r\n", "OK") {
		t.Fatal("did not expect substring match")
	}
}

func TestScorePortName(t *testing.T) {
	if ScorePortName("/dev/serial/by-id/usb-EigenComm_Compo-if03") <= ScorePortName("/dev/ttyACM0") {
		t.Fatal("expected EigenComm by-id port to score higher")
	}
	if ScorePortName("/dev/serial/by-id/usb-EigenComm_Compo-if03") <= ScorePortName("/dev/serial/by-id/usb-EigenComm_Compo-if05") {
		t.Fatal("expected if03 to score higher than if05")
	}
}

func TestScorePortNameCountsMarkerOnce(t *testing.T) {
	air780 := ScorePortName("/dev/serial/by-id/usb-Air780_Compo")
	air780e := ScorePortName("/dev/serial/by-id/usb-Air780E_Compo")
	if air780 != air780e {
		t.Fatalf("air780 score = %d, air780e score = %d, want equal marker score", air780, air780e)
	}

	multiple := ScorePortName("/dev/serial/by-id/usb-EigenComm_Air780E_Luat-if03")
	single := ScorePortName("/dev/serial/by-id/usb-EigenComm-if03")
	if multiple != single {
		t.Fatalf("multiple marker score = %d, single marker score = %d, want equal", multiple, single)
	}
}

func TestScoreLinuxTTYInfoCountsRepeatedMarkerEvidenceOnce(t *testing.T) {
	score := scoreLinuxTTYInfo(linuxTTYUSBInfo{
		Manufacturer:    "EigenComm",
		Product:         "Air780E",
		Interface:       "Luat AT",
		InterfaceNumber: 3,
	})
	want := 100 + 30
	if score != want {
		t.Fatalf("score = %d, want %d", score, want)
	}
}
