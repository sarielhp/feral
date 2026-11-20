package cli

import (
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"

	"github.com/sarielhp/feral/cache/msg"
)

func TestParseEmailDate_ZeroDateFallback(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()

	rawNoDate := []byte("Subject: No Date Email\r\n\r\nBody without date")
	if err := msg.Store(cachedir.At(tmpDir), "msg_no_date", rawNoDate, now); err != nil {
		t.Fatalf("failed to store message: %v", err)
	}

	em, raw, year, month, err := parseEmailDate(cachedir.At(tmpDir), "msg_no_date")
	if err != nil {
		t.Fatalf("parseEmailDate failed: %v", err)
	}
	if em == nil || len(raw) == 0 {
		t.Fatal("expected parsed email and raw bytes")
	}
	if em.EmailDate.IsZero() {
		t.Error("expected non-zero EmailDate after fallback")
	}
	if year == "0001" || month == "" {
		t.Errorf("expected current year/month fallback, got year=%q, month=%q", year, month)
	}
	expectedYear := now.Format("2006")
	if year != expectedYear {
		t.Errorf("expected year %q, got %q", expectedYear, year)
	}

	rawWithDate := []byte("Date: Mon, 15 Jan 2024 10:00:00 +0000\r\nSubject: Dated Email\r\n\r\nBody")
	if storeErr := msg.Store(cachedir.At(tmpDir), "msg_with_date", rawWithDate, now); storeErr != nil {
		t.Fatalf("failed to store message: %v", storeErr)
	}

	emDated, _, yearDated, monthDated, err := parseEmailDate(cachedir.At(tmpDir), "msg_with_date")
	if err != nil {
		t.Fatalf("parseEmailDate failed: %v", err)
	}
	if yearDated != "2024" || monthDated != "01" {
		t.Errorf("expected 2024/01, got %s/%s", yearDated, monthDated)
	}
	if emDated.EmailDate.Year() != 2024 {
		t.Errorf("expected parsed year 2024, got %d", emDated.EmailDate.Year())
	}
}
