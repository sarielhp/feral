package runner

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/contacts"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/report"
)

// Archiving by the short ID a listing shows must still record the sender. The
// service passed the typed reference straight to the contact bookkeeping,
// which reads the message by exact ID: a short ID found nothing, and the
// archive succeeded without the sender ever being recorded.
func TestOrganizeByID_ShortIDRecordsSender(t *testing.T) {
	root := t.TempDir()
	account := cachedir.ForAccount(root, "acct")
	id := email.ID("full-server-id-1")
	raw := []byte("From: Alice <alice@example.com>\r\nSubject: hi\r\n\r\nbody\r\n")
	if err := msg.Store(account, id, raw, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := folderindex.Add(account, "INBOX", id); err != nil {
		t.Fatal(err)
	}
	config := &cfg_g.Config{SelectedAccount: "acct", AccountCache: account}
	config.CacheRoot = root
	client := &mailclient.MockMailClient{Cfg: config}

	short := string(email.ComputeShortID(id))
	if err := (OrganizeService{Rep: report.Discard()}).ByID(config, client, label.FromServer("Archive"), short); err != nil {
		t.Fatal(err)
	}

	hb, err := contacts.LoadHistory(filepath.Join(root, "suggested_contacts.json"))
	if err != nil {
		t.Fatalf("no contact history written: %v", err)
	}
	if len(hb.Entries) != 1 || hb.Entries[0].PrimaryEmail() != "alice@example.com" {
		t.Errorf("history = %+v, want alice@example.com", hb.Entries)
	}
}
