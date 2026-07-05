package telegrambot

import (
	"fmt"
	"html"
	"strconv"
	"strings"
)

func explainCommandResult(result commandResult) []string {
	if result.Err != nil {
		return []string{fmt.Sprintf("Execution failed: <code>%s</code>", html.EscapeString(result.Err.Error()))}
	}
	if hasErrorLine(result.Lines) {
		return []string{"The module returned ERROR; the command did not complete successfully."}
	}

	switch result.Command {
	case "+CPIN?":
		return explainCPIN(result.Lines)
	case "+CSQ":
		return explainCSQ(result.Lines)
	case "+CREG?", "+CEREG?":
		return explainRegistration(result.Command, result.Lines)
	case "+COPS?":
		return explainCOPS(result.Lines)
	case "+CFUN?":
		return explainCFUN(result.Lines)
	case "+CCID":
		return explainSingleValue("ICCID", result.Lines)
	case "+CGMI":
		return explainSingleValue("Manufacturer", result.Lines)
	case "+CGMM":
		return explainSingleValue("Model", result.Lines)
	case "+CGMR":
		return explainSingleValue("Firmware version", result.Lines)
	case "+CGSN":
		return explainSingleValue("IMEI/serial number", result.Lines)
	case "+CMGF=1":
		return []string{"SMS mode has been switched to text mode."}
	case "+CNMI=2,2,0,0,0":
		return []string{"New SMS push has been configured to report directly over the serial port."}
	case "+CNMI=2,1,0,0,0":
		return []string{
			"New SMS will be stored on the SIM/module and reported as an index notification (+CMTI:).",
			"Use \"Unread SMS\" or \"All SMS\" to read them.",
			"Note: in this mode new SMS are not pushed directly, so auto-forwarding is paused until you re-enable SMS push.",
		}
	case "+CPMS?":
		return explainCPMS(result.Lines)
	case "+CMGL=\"REC UNREAD\"", "+CMGL=\"ALL\"":
		return explainCMGL(result.Lines)
	case "+RESET":
		return []string{"Restart command sent. The serial connection may drop briefly while the module reboots."}
	default:
		if okOnly(result.Lines) {
			return []string{"Command completed successfully."}
		}
		return []string{"No structured explanation is available for this response yet."}
	}
}

func explainCPIN(lines []string) []string {
	value := prefixedValue(lines, "+CPIN:")
	if value == "" {
		return []string{"No SIM status was read."}
	}
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "READY":
		return []string{"SIM status: ready."}
	case "SIM PIN":
		return []string{"SIM status: PIN unlock required."}
	case "SIM PUK":
		return []string{"SIM status: PUK unlock required."}
	default:
		return []string{fmt.Sprintf("SIM status: <code>%s</code>.", html.EscapeString(value))}
	}
}

func explainCSQ(lines []string) []string {
	values := prefixedCSV(lines, "+CSQ:")
	if len(values) < 2 {
		return []string{"No signal quality was read."}
	}
	rssi, _ := strconv.Atoi(values[0])
	ber := strings.TrimSpace(values[1])
	return []string{
		fmt.Sprintf("Signal strength: %s.", html.EscapeString(formatRSSI(rssi))),
		fmt.Sprintf("Bit error rate (BER): %s.", html.EscapeString(formatBER(ber))),
	}
}

func explainRegistration(command string, lines []string) []string {
	prefix := strings.TrimSuffix(command, "?") + ":"
	values := prefixedCSV(lines, prefix)
	if len(values) == 0 {
		return []string{"No network registration status was read."}
	}
	stat := values[0]
	if len(values) > 1 {
		stat = values[1]
	}
	name := "2G/3G network registration"
	if command == "+CEREG?" {
		name = "EPS/LTE network registration"
	}
	return []string{fmt.Sprintf("%s: %s.", name, html.EscapeString(registrationStatus(stat)))}
}

func explainCOPS(lines []string) []string {
	values := prefixedCSV(lines, "+COPS:")
	if len(values) < 3 {
		return []string{"No operator information was read."}
	}
	items := []string{
		fmt.Sprintf("Network selection mode: %s.", html.EscapeString(operatorMode(values[0]))),
		fmt.Sprintf("Operator: <code>%s</code>.", html.EscapeString(unquote(values[2]))),
	}
	if len(values) > 3 {
		items = append(items, fmt.Sprintf("Access technology: %s.", html.EscapeString(accessTechnology(values[3]))))
	}
	return items
}

func explainCFUN(lines []string) []string {
	values := prefixedCSV(lines, "+CFUN:")
	if len(values) == 0 {
		return []string{"No function mode was read."}
	}
	return []string{fmt.Sprintf("Function mode: %s.", html.EscapeString(functionMode(values[0])))}
}

func explainSingleValue(label string, lines []string) []string {
	for _, line := range responseLines(lines) {
		return []string{fmt.Sprintf("%s: <code>%s</code>.", html.EscapeString(label), html.EscapeString(line))}
	}
	return []string{fmt.Sprintf("No %s was read.", html.EscapeString(label))}
}

func explainCPMS(lines []string) []string {
	values := prefixedCSV(lines, "+CPMS:")
	if len(values) < 3 {
		return []string{"No SMS storage information was read."}
	}
	items := make([]string, 0, 3)
	labels := []string{"Read/delete storage", "Write/send storage", "Receive storage"}
	for i := 0; i+2 < len(values) && i/3 < len(labels); i += 3 {
		items = append(items, fmt.Sprintf("%s: <code>%s</code>, used %s / total %s.", labels[i/3], html.EscapeString(unquote(values[i])), html.EscapeString(values[i+1]), html.EscapeString(values[i+2])))
	}
	return items
}

func explainCMGL(lines []string) []string {
	messages := parseCMGL(lines)
	if len(messages) == 0 {
		return []string{"No SMS messages matched the query."}
	}
	items := []string{fmt.Sprintf("Read %d SMS message(s).", len(messages))}
	for _, msg := range messages {
		items = append(items, fmt.Sprintf("<b>SMS #%s</b> %s\n\n%s\n\n<b>Sender</b>: <code>%s</code>\n<b>Received</b>: <code>%s</code>", html.EscapeString(msg.index), html.EscapeString(msg.status), escapeAndTruncate(defaultText(msg.text, "(empty)"), cmglItemMax), html.EscapeString(msg.from), html.EscapeString(defaultText(msg.at, "unknown"))))
	}
	return items
}

func formatRSSI(v int) string {
	switch {
	case v == 99:
		return "unknown"
	case v == 0:
		return "very weak, about -113 dBm or lower"
	case v == 1:
		return "very weak, about -111 dBm"
	case v >= 2 && v <= 30:
		dbm := -113 + 2*v
		level := "weak"
		if v >= 20 {
			level = "good"
		} else if v >= 10 {
			level = "fair"
		}
		return fmt.Sprintf("%s, about %d dBm", level, dbm)
	case v == 31:
		return "excellent, about -51 dBm or higher"
	default:
		return fmt.Sprintf("unknown value %d", v)
	}
}

func formatBER(value string) string {
	switch strings.TrimSpace(value) {
	case "99":
		return "unknown or not detectable"
	case "0":
		return "lowest"
	case "1", "2":
		return "low"
	case "3", "4":
		return "medium"
	case "5", "6", "7":
		return "high, link quality may be poor"
	default:
		return "unknown value " + value
	}
}

func registrationStatus(value string) string {
	switch strings.TrimSpace(value) {
	case "0":
		return "not registered, not searching"
	case "1":
		return "registered on home network"
	case "2":
		return "not registered, searching"
	case "3":
		return "registration denied"
	case "4":
		return "unknown status"
	case "5":
		return "registered while roaming"
	default:
		return "unknown value " + value
	}
}

func operatorMode(value string) string {
	switch strings.TrimSpace(value) {
	case "0":
		return "automatic selection"
	case "1":
		return "manual selection"
	case "2":
		return "deregistered from network"
	case "3":
		return "set operator format only"
	case "4":
		return "manual first, automatic fallback"
	default:
		return "unknown value " + value
	}
}

func accessTechnology(value string) string {
	switch strings.TrimSpace(value) {
	case "0":
		return "GSM"
	case "2":
		return "UTRAN/3G"
	case "7":
		return "LTE/E-UTRAN"
	case "8":
		return "EC-GSM-IoT"
	case "9":
		return "NB-IoT"
	default:
		return "unknown value " + value
	}
}

func functionMode(value string) string {
	switch strings.TrimSpace(value) {
	case "0":
		return "minimum functionality"
	case "1":
		return "full functionality"
	case "4":
		return "airplane mode / RF disabled"
	default:
		return "unknown value " + value
	}
}
