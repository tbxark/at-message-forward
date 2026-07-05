package telegram

import "strings"

func prefixedValue(lines []string, prefix string) string {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func prefixedCSV(lines []string, prefix string) []string {
	value := prefixedValue(lines, prefix)
	if value == "" {
		return nil
	}
	return splitCSV(value)
}

func splitCSV(value string) []string {
	var fields []string
	var b strings.Builder
	inQuote := false
	for _, r := range value {
		switch r {
		case '"':
			inQuote = !inQuote
			b.WriteRune(r)
		case ',':
			if inQuote {
				b.WriteRune(r)
				continue
			}
			fields = append(fields, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	fields = append(fields, strings.TrimSpace(b.String()))
	return fields
}

func responseLines(lines []string) []string {
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "OK" || line == "ERROR" {
			continue
		}
		clean = append(clean, line)
	}
	return clean
}

func hasErrorLine(lines []string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) == "ERROR" {
			return true
		}
	}
	return false
}

func okOnly(lines []string) bool {
	seenOK := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line != "OK" {
			return false
		}
		seenOK = true
	}
	return seenOK
}

func unquote(value string) string {
	return strings.Trim(strings.TrimSpace(value), "\"")
}

type listedSMS struct {
	index  string
	status string
	from   string
	at     string
	text   string
}

func parseCMGL(lines []string) []listedSMS {
	var messages []listedSMS
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "+CMGL:") {
			continue
		}
		values := splitCSV(strings.TrimSpace(strings.TrimPrefix(line, "+CMGL:")))
		msg := listedSMS{index: "?", status: "unknown", from: "unknown"}
		if len(values) > 0 {
			msg.index = values[0]
		}
		if len(values) > 1 {
			msg.status = unquote(values[1])
		}
		if len(values) > 2 {
			msg.from = unquote(values[2])
		}
		if len(values) > 4 {
			msg.at = unquote(values[4])
		}
		if i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			if next != "" && !strings.HasPrefix(next, "+") && next != "OK" && next != "ERROR" {
				msg.text = next
			}
		}
		messages = append(messages, msg)
	}
	return messages
}
