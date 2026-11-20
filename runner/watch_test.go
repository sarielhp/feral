package runner

import (
	"slices"
	"testing"

	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

// The watcher's archive of a muted thread's mail is a move from the inbox to
// the account's archive folder, the same one `E` and `msg archive` use.
func TestArchiveFromInbox(t *testing.T) {
	client := &mailclient.MockMailClient{
		Cfg:            &cfg_g.Config{},
		InboxFolderVal: "INBOX",
		Labels:         []string{"INBOX", "Archive"},
	}
	if err := archiveFromInbox(client, []email.ID{"m1", "m2"}); err != nil {
		t.Fatalf("archiveFromInbox: %v", err)
	}
	if len(client.MovedEmails) != 1 {
		t.Fatalf("moves = %+v, want one", client.MovedEmails)
	}
	move := client.MovedEmails[0]
	if move.SourceLabel != "INBOX" || move.DestLabel != "Archive" || !slices.Equal(move.MessageIDs, []email.ID{"m1", "m2"}) {
		t.Errorf("move = %+v, want m1 and m2 from INBOX to Archive", move)
	}
}
