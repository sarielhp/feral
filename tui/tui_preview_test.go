package tui

import (
	"testing"

	"github.com/sarielhp/feral/email"
)

func TestIsImageAttachment(t *testing.T) {
	cases := []struct {
		filename    string
		contentType string
		data        []byte
		want        bool
	}{
		{"photo.png", "image/png", nil, true},
		{"photo.jpg", "", nil, true},
		{"photo.jpeg", "", nil, true},
		{"anim.gif", "image/gif", nil, true},
		{"vector.svg", "", nil, true},
		{"unknown.bin", "", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, true},
		{"unknown.bin", "", []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46}, true},
		{"doc.pdf", "application/pdf", []byte("%PDF-1.4"), false},
		{"notes.txt", "text/plain", []byte("hello world"), false},
	}

	for _, tc := range cases {
		att := email.Attachment{
			Filename:    tc.filename,
			ContentType: tc.contentType,
			Data:        tc.data,
		}
		if got := isImageAttachment(att); got != tc.want {
			t.Errorf("isImageAttachment(%q, %q) = %v, want %v", tc.filename, tc.contentType, got, tc.want)
		}
	}
}
