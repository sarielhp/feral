package runner

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/cli"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/report"
)

// `msg reply --list` addresses the list's List-Post alone -- not the sender's
// Reply-To, not the Cc -- and refuses a message from no list or a closed one
// rather than mailing the sender instead.
func TestReplyService_ListReply(t *testing.T) {
	listRaw := "From: Alice <alice@example.com>\r\nTo: dev@lists.example.org\r\n" +
		"Cc: Carol <carol@example.com>\r\nReply-To: alice@home.example\r\n" +
		"Subject: [dev] plan\r\nMessage-ID: <plan@example.com>\r\n" +
		"List-Id: <dev.lists.example.org>\r\nList-Post: <mailto:dev@lists.example.org>\r\n" +
		"Date: Sun, 20 Sep 2026 11:00:00 +0000\r\n\r\nThe plan.\r\n"
	tests := []struct {
		name    string
		raw     string
		wantErr error
	}{
		{"to the list", listRaw, nil},
		{"no List-Post", strings.Replace(listRaw, "List-Post: <mailto:dev@lists.example.org>\r\n", "", 1), email.ErrNoListPost},
		{"closed list", strings.Replace(listRaw, "<mailto:dev@lists.example.org>", "NO", 1), email.ErrListClosed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			setupTestCachedMessage(t, dir, "work", "list01", []byte(tc.raw))
			var out bytes.Buffer
			err := SendService{Rep: report.Discard(), Out: &out}.Reply(&cfg_g.Config{
				Paths:    cfg_g.Paths{ConfigDir: dir, CacheRoot: dir},
				Accounts: []cfg_acc.AccountConfig{{Name: "work", Username: "me@work.example", Type: "jmap"}},
			}, cli.ReplyOptions{MessageID: "list01", List: true, Headless: true, DryRun: true, Body: "Agreed."})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Reply error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if out.Len() != 0 {
					t.Errorf("a refused list reply still produced a message:\n%s", out.String())
				}
				return
			}
			got := out.String()
			if !strings.Contains(got, "To: dev@lists.example.org") {
				t.Errorf("want the reply addressed to the list, got:\n%s", got)
			}
			if strings.Contains(got, "alice@home.example") || strings.Contains(got, "carol@example.com") {
				t.Errorf("a list reply copied the sender or the Cc:\n%s", got)
			}
			if !strings.Contains(got, "In-Reply-To: <plan@example.com>") {
				t.Errorf("want the threading headers, got:\n%s", got)
			}
		})
	}
}
