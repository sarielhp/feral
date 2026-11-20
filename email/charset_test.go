package email

import (
	"bytes"
	"net/mail"
	"os/exec"
	"strings"
	"testing"
)

// hebrewQP is "שלום עולם" in windows-1255, quoted-printable.
const hebrewQP = "=F9=EC=E5=ED =F2=E5=EC=ED"

const hebrewText = "שלום עולם"

// outlookHebrew is the shape Outlook sends Hebrew in: multipart/alternative,
// both parts windows-1255, quoted-printable, the HTML declaring its charset in
// a <meta> as well.
const outlookHebrew = "From: a@example.com\r\n" +
	"Subject: test\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/alternative; boundary=\"b\"\r\n" +
	"\r\n" +
	"--b\r\n" +
	"Content-Type: text/plain; charset=\"windows-1255\"\r\n" +
	"Content-Transfer-Encoding: quoted-printable\r\n" +
	"\r\n" +
	hebrewQP + "\r\n" +
	"--b\r\n" +
	"Content-Type: text/html; charset=\"windows-1255\"\r\n" +
	"Content-Transfer-Encoding: quoted-printable\r\n" +
	"\r\n" +
	"<html><head><meta http-equiv=3D\"Content-Type\" content=3D\"text/html; charset=3Dwindows-1255\"></head>" +
	"<body><p>" + hebrewQP + "</p></body></html>\r\n" +
	"--b--\r\n"

func TestTextToUTF8(t *testing.T) {
	win1255 := []byte{0xF9, 0xEC, 0xE5, 0xED}
	tests := []struct {
		name        string
		data        []byte
		contentType string
		want        string
	}{
		{"windows-1255 plain", win1255, `text/plain; charset="windows-1255"`, "שלום"},
		{"iso-8859-8 plain", win1255, "text/plain; charset=iso-8859-8", "שלום"},
		{"latin1 plain", []byte{'c', 'a', 'f', 0xE9}, "text/plain; charset=ISO-8859-1", "café"},
		{"utf-8 untouched", []byte("שלום"), "text/plain; charset=utf-8", "שלום"},
		{"no charset untouched", []byte("שלום"), "text/plain", "שלום"},
		{"unknown charset untouched", []byte("abc"), "text/plain; charset=x-nonsense", "abc"},
		{
			"html charset from meta, meta rewritten",
			append([]byte(`<meta charset="windows-1255"><p>`), win1255...),
			"text/html",
			`<meta charset="utf-8"><p>שלום`,
		},
		{
			"html charset from header, http-equiv rewritten",
			append([]byte(`<meta http-equiv="Content-Type" content="text/html; charset=windows-1255">`), win1255...),
			"text/html; charset=windows-1255",
			`<meta http-equiv="Content-Type" content="text/html; charset=utf-8">שלום`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(TextToUTF8(tt.data, tt.contentType)); got != tt.want {
				t.Errorf("TextToUTF8() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractPlainBodyText_Windows1255(t *testing.T) {
	msg, err := mail.ReadMessage(strings.NewReader(outlookHebrew))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExtractPlainBodyText(msg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != hebrewText {
		t.Errorf("body = %q, want %q", got, hebrewText)
	}
}

func TestExtractAlternativeBodyParts_Windows1255(t *testing.T) {
	plain, html, err := ExtractAlternativeBodyParts([]byte(outlookHebrew))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(plain) != hebrewText {
		t.Errorf("plain = %q, want %q", plain, hebrewText)
	}
	if !strings.Contains(html, hebrewText) || !strings.Contains(html, "charset=utf-8") {
		t.Errorf("html = %q, want UTF-8 Hebrew and a utf-8 meta", html)
	}
}

// An HTML-only message goes through w3m; with the meta still naming the old
// charset, w3m would decode the converted UTF-8 a second time.
func TestExtractPlainBodyText_HTMLOnlyWindows1255(t *testing.T) {
	if _, err := exec.LookPath("w3m"); err != nil {
		t.Skip("w3m not installed")
	}
	raw := "Content-Type: text/html; charset=windows-1255\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n\r\n" +
		"<html><head><meta charset=3D\"windows-1255\"></head><body>" + hebrewQP + "</body></html>\r\n"
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExtractPlainBodyText(msg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, hebrewText) {
		t.Errorf("body = %q, want it to contain %q", got, hebrewText)
	}
}

func TestExtractPlainBodyTextFromMsg_Windows1255(t *testing.T) {
	msg, err := mail.ReadMessage(bytes.NewReader([]byte(outlookHebrew)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := extractPlainBodyTextFromMsg(msg.Header, msg.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != hebrewText {
		t.Errorf("quoted body = %q, want %q", got, hebrewText)
	}
}
