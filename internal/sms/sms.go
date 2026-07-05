package sms

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

type Event struct {
	From string
	Text string
	At   time.Time
}

var (
	cmtHeaderRE    = regexp.MustCompile(`^\+CMT:\s*"([^"]*)"`)
	cmtPDUHeaderRE = regexp.MustCompile(`^\+CMT:\s*(?:(?:"[^"]*"|[^,]*))?\s*,\s*(\d+)\s*$`)
	hexLineRE      = regexp.MustCompile(`^[0-9A-Fa-f]+$`)
)

var gsm7DefaultAlphabet = [128]rune{
	'@', '£', '$', '¥', 'è', 'é', 'ù', 'ì', 'ò', 'Ç', '\n', 'Ø', 'ø', '\r', 'Å', 'å',
	'Δ', '_', 'Φ', 'Γ', 'Λ', 'Ω', 'Π', 'Ψ', 'Σ', 'Θ', 'Ξ', '\u001b', 'Æ', 'æ', 'ß', 'É',
	' ', '!', '"', '#', '¤', '%', '&', '\'', '(', ')', '*', '+', ',', '-', '.', '/',
	'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', ':', ';', '<', '=', '>', '?',
	'¡', 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O',
	'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z', 'Ä', 'Ö', 'Ñ', 'Ü', '§',
	'¿', 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o',
	'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z', 'ä', 'ö', 'ñ', 'ü', 'à',
}

func ParseCMTIndication(lines []string) (Event, error) {
	var event Event
	if len(lines) < 2 {
		return event, fmt.Errorf("missing message body")
	}

	header := strings.TrimSpace(lines[0])
	body := strings.TrimSpace(lines[1])

	if m := cmtPDUHeaderRE.FindStringSubmatch(header); len(m) == 2 {
		if !hexLineRE.MatchString(body) {
			return event, fmt.Errorf("pdu body is not hex")
		}
		length, err := strconv.Atoi(m[1])
		if err != nil {
			return event, fmt.Errorf("parse pdu length: %w", err)
		}
		return DecodePDU(body, length)
	}

	if m := cmtHeaderRE.FindStringSubmatch(header); len(m) == 2 {
		return Event{From: m[1], Text: body, At: time.Now()}, nil
	}

	return event, fmt.Errorf("unsupported +CMT header: %s", header)
}

func DecodePDU(pdu string, expectedTPDULength int) (Event, error) {
	var event Event
	event.At = time.Now()

	pdu = strings.TrimSpace(pdu)
	if len(pdu)%2 != 0 {
		return event, fmt.Errorf("odd pdu hex length")
	}

	pos := 0
	readHex := func(n int) (string, error) {
		if pos+n > len(pdu) {
			return "", io.ErrUnexpectedEOF
		}
		s := pdu[pos : pos+n]
		pos += n
		return s, nil
	}
	readByte := func() (byte, error) {
		s, err := readHex(2)
		if err != nil {
			return 0, err
		}
		v, err := strconv.ParseUint(s, 16, 8)
		if err != nil {
			return 0, err
		}
		return byte(v), nil
	}

	smscLen, err := readByte()
	if err != nil {
		return event, err
	}
	if _, err := readHex(int(smscLen) * 2); err != nil {
		return event, err
	}
	if expectedTPDULength >= 0 {
		actualTPDULength := (len(pdu) - pos) / 2
		if actualTPDULength != expectedTPDULength {
			return event, fmt.Errorf("tpdu length mismatch: header=%d actual=%d", expectedTPDULength, actualTPDULength)
		}
	}

	firstOctet, err := readByte()
	if err != nil {
		return event, err
	}
	mti := firstOctet & 0x03
	switch mti {
	case 0x00:
		return decodeDeliverTPDU(event, firstOctet, readByte, readHex)
	case 0x01:
		return decodeSubmitTPDU(event, firstOctet, readByte, readHex)
	default:
		return event, fmt.Errorf("unsupported tpdu mti=0x%02X", mti)
	}
}

func decodeDeliverTPDU(event Event, firstOctet byte, readByte func() (byte, error), readHex func(int) (string, error)) (Event, error) {
	hasUDH := firstOctet&0x40 != 0

	originLen, err := readByte()
	if err != nil {
		return event, err
	}
	originTOA, err := readByte()
	if err != nil {
		return event, err
	}
	originHexLen := int((originLen + 1) / 2 * 2)
	originRaw, err := readHex(originHexLen)
	if err != nil {
		return event, err
	}
	event.From = decodeSemiOctets(originRaw, int(originLen))
	if originTOA == 0x91 {
		event.From = "+" + event.From
	}

	if _, err := readByte(); err != nil {
		return event, err
	}
	dcs, err := readByte()
	if err != nil {
		return event, err
	}
	if _, err := readHex(14); err != nil {
		return event, err
	}
	userDataLen, err := readByte()
	if err != nil {
		return event, err
	}
	userData, err := readHexRemaining(readHex)
	if err != nil {
		return event, err
	}

	text, err := decodeUserData(dcs, hasUDH, int(userDataLen), userData)
	if err != nil {
		return event, err
	}
	event.Text = text
	return event, nil
}

func decodeSubmitTPDU(event Event, firstOctet byte, readByte func() (byte, error), readHex func(int) (string, error)) (Event, error) {
	hasUDH := firstOctet&0x40 != 0
	if _, err := readByte(); err != nil { // MR
		return event, err
	}
	destinationLen, err := readByte()
	if err != nil {
		return event, err
	}
	destinationTOA, err := readByte()
	if err != nil {
		return event, err
	}
	destinationHexLen := int((destinationLen + 1) / 2 * 2)
	destinationRaw, err := readHex(destinationHexLen)
	if err != nil {
		return event, err
	}
	event.From = decodeSemiOctets(destinationRaw, int(destinationLen))
	if destinationTOA == 0x91 {
		event.From = "+" + event.From
	}
	if _, err := readByte(); err != nil { // PID
		return event, err
	}
	dcs, err := readByte()
	if err != nil {
		return event, err
	}
	if err := skipSubmitValidityPeriod(firstOctet, readHex); err != nil {
		return event, err
	}
	userDataLen, err := readByte()
	if err != nil {
		return event, err
	}
	userData, err := readHexRemaining(readHex)
	if err != nil {
		return event, err
	}
	text, err := decodeUserData(dcs, hasUDH, int(userDataLen), userData)
	if err != nil {
		return event, err
	}
	event.Text = text
	return event, nil
}

func skipSubmitValidityPeriod(firstOctet byte, readHex func(int) (string, error)) error {
	switch firstOctet & 0x18 {
	case 0x00:
		return nil
	case 0x10:
		_, err := readHex(2)
		return err
	case 0x08, 0x18:
		_, err := readHex(14)
		return err
	default:
		return nil
	}
}

func readHexRemaining(readHex func(int) (string, error)) (string, error) {
	var b strings.Builder
	for {
		s, err := readHex(2)
		if err == io.ErrUnexpectedEOF {
			return b.String(), nil
		}
		if err != nil {
			return "", err
		}
		b.WriteString(s)
	}
}

func decodeSemiOctets(hex string, digits int) string {
	var b strings.Builder
	for i := 0; i+1 < len(hex); i += 2 {
		b.WriteByte(hex[i+1])
		b.WriteByte(hex[i])
	}
	out := strings.TrimRight(b.String(), "Ff")
	if digits > 0 && len(out) > digits {
		return out[:digits]
	}
	return out
}

func decodeUserData(dcs byte, hasUDH bool, userDataLen int, userData string) (string, error) {
	switch dcs {
	case 0x00:
		return decodeGSM7(userData, userDataLen, hasUDH)
	case 0x08:
		return decodeUCS2(userData, hasUDH)
	case 0x04:
		return "[8-bit user data] " + userData, nil
	default:
		return fmt.Sprintf("[unsupported dcs=0x%02X] %s", dcs, userData), nil
	}
}

func decodeGSM7(hex string, septets int, hasUDH bool) (string, error) {
	data, err := hexToBytes(hex)
	if err != nil {
		return "", err
	}
	skipBits := 0
	if hasUDH {
		if len(data) == 0 {
			return "", io.ErrUnexpectedEOF
		}
		headerBytes := int(data[0]) + 1
		if headerBytes > len(data) {
			return "", io.ErrUnexpectedEOF
		}
		skipBits = headerBytes * 8
	}

	var b strings.Builder
	for septet := 0; septet < septets; septet++ {
		bitOffset := skipBits + septet*7
		value := byte(0)
		for bit := 0; bit < 7; bit++ {
			absolute := bitOffset + bit
			byteIndex := absolute / 8
			if byteIndex >= len(data) {
				break
			}
			if data[byteIndex]&(1<<uint(absolute%8)) != 0 {
				value |= 1 << uint(bit)
			}
		}
		b.WriteRune(gsm7Rune(value))
	}
	return b.String(), nil
}

func decodeUCS2(hex string, hasUDH bool) (string, error) {
	data, err := hexToBytes(hex)
	if err != nil {
		return "", err
	}
	if hasUDH {
		if len(data) == 0 {
			return "", io.ErrUnexpectedEOF
		}
		headerBytes := int(data[0]) + 1
		if headerBytes > len(data) {
			return "", io.ErrUnexpectedEOF
		}
		data = data[headerBytes:]
	}
	if len(data)%2 != 0 {
		return "", fmt.Errorf("odd ucs2 byte length")
	}
	words := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		words = append(words, uint16(data[i])<<8|uint16(data[i+1]))
	}
	return string(utf16.Decode(words)), nil
}

func hexToBytes(hex string) ([]byte, error) {
	data := make([]byte, 0, len(hex)/2)
	for i := 0; i+1 < len(hex); i += 2 {
		v, err := strconv.ParseUint(hex[i:i+2], 16, 8)
		if err != nil {
			return nil, err
		}
		data = append(data, byte(v))
	}
	return data, nil
}

func gsm7Rune(v byte) rune {
	if int(v) >= len(gsm7DefaultAlphabet) {
		return '?'
	}
	return gsm7DefaultAlphabet[v]
}
