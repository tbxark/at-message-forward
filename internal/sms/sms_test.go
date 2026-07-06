package sms

import "testing"

func TestDecodePDU_GSM7(t *testing.T) {
	event, err := DecodePDU("07915892200417F5240BA14180889969F100006260918022320004F4F29C0E", 23)
	if err != nil {
		t.Fatal(err)
	}
	if event.From != "14088899961" {
		t.Fatalf("from = %q", event.From)
	}
	if event.Text != "test" {
		t.Fatalf("text = %q", event.Text)
	}
}

func TestDecodePDURejectsLengthMismatch(t *testing.T) {
	_, err := DecodePDU("07915892200417F5240BA14180889969F100006260918022320004F4F29C0E", 24)
	if err == nil {
		t.Fatal("expected length mismatch error")
	}
}

func TestDecodePDU_SMSSubmitWithRelativeValidityPeriod(t *testing.T) {
	event, err := DecodePDU("0011000B912143658709F10000AA04F4F29C0E", 18)
	if err != nil {
		t.Fatal(err)
	}
	if event.From != "+12345678901" {
		t.Fatalf("from = %q", event.From)
	}
	if event.Text != "test" {
		t.Fatalf("text = %q", event.Text)
	}
}

func TestDecodePDURejectsUnsupportedMTI(t *testing.T) {
	_, err := DecodePDU("0002", 1)
	if err == nil {
		t.Fatal("expected unsupported MTI error")
	}
}

func TestCMTPDUHeaderRegex(t *testing.T) {
	cases := map[string]string{
		`+CMT:,23`:       "23",
		`+CMT: , 23`:     "23",
		`+CMT:"",23`:     "23",
		`+CMT:"alpha",5`: "5",
	}
	for line, want := range cases {
		t.Run(line, func(t *testing.T) {
			m := cmtPDUHeaderRE.FindStringSubmatch(line)
			if len(m) != 2 {
				t.Fatalf("expected match, got %v", m)
			}
			if m[1] != want {
				t.Fatalf("length = %q", m[1])
			}
		})
	}

	if cmtPDUHeaderRE.MatchString(`+CMT: "+86138xxxx0000","","26/06/19,16:30:00+32"`) {
		t.Fatal("text-mode CMT header should not match PDU header")
	}
}

func TestParseCMTIndicationText(t *testing.T) {
	event, err := ParseCMTIndication([]string{
		`+CMT: "+86138xxxx0000","","26/06/19,16:30:00+32"`,
		"Your code is 123456",
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.From != "+86138xxxx0000" {
		t.Fatalf("from = %q", event.From)
	}
	if event.Text != "Your code is 123456" {
		t.Fatalf("text = %q", event.Text)
	}
}

func TestParseCMTIndicationTextUCS2Hex(t *testing.T) {
	// Text mode (+CMGF=1) with a GSM charset: the phone number in the header is
	// plain ASCII, but non-GSM content (Chinese) is delivered as raw UCS2 hex.
	event, err := ParseCMTIndication([]string{
		`+CMT: "10686230813854910496",,"26/07/06,13:56:07+32"`,
		"3010003500320054004F00590053301160A876849A8C8BC17801662F003700390031003930025982975E672C4EBA64CD4F5CFF0C8BF75FFD7565672C77ED4FE1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.From != "10686230813854910496" {
		t.Fatalf("from = %q", event.From)
	}
	if want := "【52TOYS】您的验证码是7919。如非本人操作，请忽略本短信"; event.Text != want {
		t.Fatalf("text = %q, want %q", event.Text, want)
	}
}

func TestParseCMTIndicationPDU(t *testing.T) {
	event, err := ParseCMTIndication([]string{
		`+CMT:,23`,
		"07915892200417F5240BA14180889969F100006260918022320004F4F29C0E",
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.From != "14088899961" {
		t.Fatalf("from = %q", event.From)
	}
	if event.Text != "test" {
		t.Fatalf("text = %q", event.Text)
	}
}
