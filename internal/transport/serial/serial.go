package serial

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	bugst "go.bug.st/serial"
)

type Candidate struct {
	Port           string
	Source         string
	Score          int
	ProbeAttempted bool
	ProbeOK        bool
	ProbeError     string
}

const (
	DefaultProbeTimeout = 1500 * time.Millisecond
	defaultProbeBaud    = 115200
)

var (
	candidateProvider = Candidates
	probeSerialPort   = ProbeAT
	// Seams for the Transport open path, overridable in tests.
	autoDetectPort = AutoDetectWithBaud
	openPort       = Open
)

// Transport opens a modem's AT stream over a serial port. When Port is empty,
// each Open re-runs auto-detection so a re-enumerated USB device is found on
// reconnect. It implements the parent transport.Transport interface.
type Transport struct {
	Port string
	Baud int
}

// New returns a serial Transport for the given port (empty = auto-detect) and
// baud rate.
func New(port string, baud int) *Transport {
	return &Transport{Port: port, Baud: baud}
}

// Open resolves the port (auto-detecting when unset), opens it, and returns the
// AT stream plus the resolved port name.
func (t *Transport) Open(_ context.Context) (io.ReadWriteCloser, string, error) {
	portName := t.Port
	if portName == "" {
		detected, err := autoDetectPort(t.Baud)
		if err != nil {
			return nil, "", fmt.Errorf("serial port not found: %w", err)
		}
		portName = detected
	}

	slog.Info("using serial port", "port", portName, "baud", t.Baud)
	port, err := openPort(portName, t.Baud)
	if err != nil {
		return nil, "", fmt.Errorf("open serial failed: %w", err)
	}
	return port, portName, nil
}

func AutoDetect() (string, error) {
	return AutoDetectWithBaud(defaultProbeBaud)
}

func AutoDetectWithBaud(baud int) (string, error) {
	candidates := candidateProvider()
	if len(candidates) == 0 {
		return "", fmt.Errorf("no matching ports")
	}
	probed := ProbeCandidates(candidates, baud, DefaultProbeTimeout)
	selected, ok := SelectAutoDetectCandidate(probed)
	if !ok {
		return "", fmt.Errorf("no matching ports")
	}
	return selected.Port, nil
}

func Open(portName string, baud int) (bugst.Port, error) {
	if baud <= 0 {
		return nil, fmt.Errorf("invalid baud %d", baud)
	}
	return bugst.Open(portName, &bugst.Mode{
		BaudRate: baud,
		DataBits: 8,
		Parity:   bugst.NoParity,
		StopBits: bugst.OneStopBit,
	})
}

func PrintCandidates() {
	candidates := Candidates()
	printCandidates(candidates, "")
}

func PrintProbedCandidates(baud int, timeout time.Duration) {
	candidates := ProbeCandidates(Candidates(), baud, timeout)
	selected, _ := SelectAutoDetectCandidate(candidates)
	printCandidates(candidates, selected.Port)
}

func printCandidates(candidates []Candidate, selectedPort string) {
	if len(candidates) == 0 {
		slog.Warn("no serial candidates found")
		return
	}

	for _, candidate := range candidates {
		args := []any{"port", candidate.Port, "score", candidate.Score, "source", candidate.Source}
		if selectedPort != "" {
			args = append(args, "auto", candidate.Port == selectedPort)
		}
		if candidate.ProbeAttempted {
			args = append(args, "probe", candidateProbeStatus(candidate))
			if candidate.ProbeError != "" {
				args = append(args, "probe_error", candidate.ProbeError)
			}
		}
		slog.Info("serial candidate", args...)
	}
}

func Candidates() []Candidate {
	var candidates []Candidate
	if runtime.GOOS == "linux" {
		candidates = append(candidates, linuxSerialByIDCandidates()...)
	}
	candidates = append(candidates, serialLibraryCandidates()...)

	return RankCandidates(candidates)
}

func serialLibraryCandidates() []Candidate {
	ports, err := bugst.GetPortsList()
	if err != nil {
		slog.Error("list serial ports failed", "err", err)
		return nil
	}
	var candidates []Candidate
	for _, port := range ports {
		candidates = append(candidates, Candidate{
			Port:   port,
			Source: "serial-list",
			Score:  10 + ScorePortName(port) + scoreLinuxTTY(port),
		})
	}
	return candidates
}

func linuxSerialByIDCandidates() []Candidate {
	matches, _ := filepath.Glob("/dev/serial/by-id/*")
	var candidates []Candidate
	for _, link := range matches {
		port, err := filepath.EvalSymlinks(link)
		if err != nil {
			continue
		}
		candidates = append(candidates, Candidate{
			Port:   port,
			Source: "by-id:" + filepath.Base(link),
			Score:  ScorePortName(link) + scoreLinuxTTY(port) + 50,
		})
	}
	return candidates
}

func RankCandidates(candidates []Candidate) []Candidate {
	best := make(map[string]Candidate)
	for _, candidate := range candidates {
		if candidate.Port == "" {
			continue
		}
		if existing, ok := best[candidate.Port]; !ok || candidate.Score > existing.Score {
			best[candidate.Port] = candidate
		}
	}

	ranked := make([]Candidate, 0, len(best))
	for _, candidate := range best {
		ranked = append(ranked, candidate)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		return ranked[i].Port < ranked[j].Port
	})
	return ranked
}

func ProbeCandidates(candidates []Candidate, baud int, timeout time.Duration) []Candidate {
	probed := make([]Candidate, len(candidates))
	for i, candidate := range candidates {
		candidate.ProbeAttempted = true
		candidate.ProbeOK = false
		candidate.ProbeError = ""
		ok, err := probeSerialPort(candidate.Port, baud, timeout)
		candidate.ProbeOK = ok
		if err != nil {
			candidate.ProbeError = err.Error()
		}
		probed[i] = candidate
	}
	sort.SliceStable(probed, func(i, j int) bool {
		return probed[i].ProbeOK && !probed[j].ProbeOK
	})
	return probed
}

func SelectAutoDetectCandidate(candidates []Candidate) (Candidate, bool) {
	for _, candidate := range candidates {
		if candidate.ProbeOK {
			return candidate, true
		}
	}
	if len(candidates) == 0 {
		return Candidate{}, false
	}
	return candidates[0], true
}

func ProbeAT(portName string, baud int, timeout time.Duration) (bool, error) {
	if timeout <= 0 {
		timeout = DefaultProbeTimeout
	}
	port, err := Open(portName, baud)
	if err != nil {
		return false, err
	}
	defer func() {
		if err := port.Close(); err != nil {
			slog.Warn("serial probe close failed", "port", portName, "err", err)
		}
	}()
	return probeOpenedATPort(port, timeout)
}

func probeOpenedATPort(port bugst.Port, timeout time.Duration) (bool, error) {
	if timeout <= 0 {
		timeout = DefaultProbeTimeout
	}
	if err := port.SetReadTimeout(minDuration(100*time.Millisecond, timeout)); err != nil {
		return false, fmt.Errorf("set read timeout: %w", err)
	}
	if err := port.ResetInputBuffer(); err != nil {
		slog.Debug("serial probe reset input failed", "err", err)
	}
	if err := port.ResetOutputBuffer(); err != nil {
		slog.Debug("serial probe reset output failed", "err", err)
	}
	probeCommand := []byte("AT\r")
	if n, err := port.Write(probeCommand); err != nil {
		return false, fmt.Errorf("write AT: %w", err)
	} else if n != len(probeCommand) {
		return false, fmt.Errorf("write AT: short write %d/%d", n, len(probeCommand))
	}

	deadline := time.Now().Add(timeout)
	buf := make([]byte, 128)
	var response strings.Builder
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if err := port.SetReadTimeout(minDuration(100*time.Millisecond, remaining)); err != nil {
			return false, fmt.Errorf("set read timeout: %w", err)
		}
		n, err := port.Read(buf)
		if n > 0 {
			response.Write(buf[:n])
			if responseHasATLine(response.String(), "OK") {
				return true, nil
			}
			if responseHasATLine(response.String(), "ERROR") {
				return false, fmt.Errorf("AT returned ERROR")
			}
		}
		if err != nil {
			return false, fmt.Errorf("read AT response: %w", err)
		}
	}
	if response.Len() > 0 {
		return false, fmt.Errorf("AT response without OK: %q", strings.TrimSpace(response.String()))
	}
	return false, fmt.Errorf("AT probe timeout")
}

func responseHasATLine(response string, target string) bool {
	for _, line := range strings.FieldsFunc(response, func(r rune) bool {
		return r == '\r' || r == '\n'
	}) {
		if strings.EqualFold(strings.TrimSpace(line), target) {
			return true
		}
	}
	return false
}

func candidateProbeStatus(candidate Candidate) string {
	if candidate.ProbeOK {
		return "ok"
	}
	return "failed"
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func ScorePortName(path string) int {
	name := strings.ToLower(filepath.Base(path))
	score := scoreMarkerEvidence(name)
	if strings.Contains(name, "if03") {
		score += 30
	} else if strings.Contains(name, "if05") {
		score += 10
	}
	if strings.Contains(name, "ttyacm") || strings.Contains(name, "usbmodem") {
		score += 10
	}
	return score
}

// modemNameMarkers are lowercase substrings commonly found in the USB
// product/interface names of AT-capable cellular modems. Matching any one of
// them boosts a port's score. Add new vendor/model markers here as needed.
var modemNameMarkers = []string{
	// EigenComm / Luat Air series (e.g. Air780E)
	"eigencomm", "air780", "luat",
	// SIMCom (SIM7600/SIM7000/SIM800, A7670, ...)
	"simcom", "sim7600", "sim7000", "sim800", "a7670",
	// Quectel (EC20/EC25/EG25, BG96, ...)
	"quectel", "ec20", "ec25", "eg25", "bg96",
	// Other common cellular modem vendors
	"fibocom", "telit", "u-blox", "ublox", "meig", "ml307", "gosuncn",
}

func scoreMarkerEvidence(values ...string) int {
	for _, value := range values {
		lower := strings.ToLower(value)
		for _, marker := range modemNameMarkers {
			if strings.Contains(lower, marker) {
				return 100
			}
		}
	}
	return 0
}

func scoreLinuxTTY(port string) int {
	if runtime.GOOS != "linux" {
		return 0
	}

	return scoreLinuxTTYInfo(readLinuxTTYUSBInfo(filepath.Base(port)))
}

func scoreLinuxTTYInfo(info linuxTTYUSBInfo) int {
	score := 0
	score += scoreMarkerEvidence(info.Manufacturer, info.Product, info.Interface)
	if strings.EqualFold(info.VendorID, "19d1") && strings.EqualFold(info.ProductID, "0001") {
		score += 150
	}
	switch strings.ToLower(info.Interface) {
	case "at", "usb uart":
		score += 40
	case "log":
		score -= 30
	case "ppp", "rndis":
		score -= 50
	}
	if info.InterfaceNumber == 3 {
		score += 30
	} else if info.InterfaceNumber == 5 {
		score += 10
	} else if info.InterfaceNumber > 5 {
		score -= info.InterfaceNumber
	}
	return score
}

type linuxTTYUSBInfo struct {
	VendorID        string
	ProductID       string
	Manufacturer    string
	Product         string
	Interface       string
	InterfaceNumber int
}

func readLinuxTTYUSBInfo(tty string) linuxTTYUSBInfo {
	var info linuxTTYUSBInfo
	path := filepath.Join("/sys/class/tty", tty, "device")
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return info
	}

	for dir := realPath; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		info.VendorID = firstNonEmpty(info.VendorID, readTrimmed(filepath.Join(dir, "idVendor")))
		info.ProductID = firstNonEmpty(info.ProductID, readTrimmed(filepath.Join(dir, "idProduct")))
		info.Manufacturer = firstNonEmpty(info.Manufacturer, readTrimmed(filepath.Join(dir, "manufacturer")))
		info.Product = firstNonEmpty(info.Product, readTrimmed(filepath.Join(dir, "product")))
		info.Interface = firstNonEmpty(info.Interface, readTrimmed(filepath.Join(dir, "interface")))
		if info.InterfaceNumber == 0 {
			if value := readTrimmed(filepath.Join(dir, "bInterfaceNumber")); value != "" {
				if number, err := strconv.Atoi(value); err == nil {
					info.InterfaceNumber = number
				}
			}
		}
		if info.VendorID != "" && info.ProductID != "" && info.Interface != "" {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return info
}

func readTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
