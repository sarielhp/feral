package gmail

import (
	"errors"
	"slices"
	"testing"
)

// fakeLabelIDs resolves the labels an account has, as the API would.
func fakeLabelIDs(known map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if id, ok := known[name]; ok {
			return id, nil
		}
		return "", errors.New("label not found")
	}
}

// A missing "Archive" label is no label on either side of a move: archiving
// removes the source label alone, and moving the message back -- what undo
// does -- adds the destination label alone. It used to be special only as a
// destination, so the move back failed to resolve its source.
func TestMoveLabelChange(t *testing.T) {
	withArchive := map[string]string{"INBOX": "INBOX", "Archive": "Label_9", "TRASH": "TRASH", "SENT": "SENT"}
	noArchive := map[string]string{"INBOX": "INBOX", "TRASH": "TRASH"}
	tests := []struct {
		name               string
		known              map[string]string
		source, dest       string
		wantAdd, wantRemov []string
		wantErr            bool
	}{
		{name: "archive without the label", known: noArchive, source: "INBOX", dest: "Archive", wantRemov: []string{"INBOX"}},
		{name: "back from archive without the label", known: noArchive, source: "Archive", dest: "INBOX", wantAdd: []string{"INBOX"}},
		{name: "archive with the label", known: withArchive, source: "INBOX", dest: "Archive", wantAdd: []string{"Label_9"}, wantRemov: []string{"INBOX"}},
		{name: "back from archive with the label", known: withArchive, source: "Archive", dest: "INBOX", wantAdd: []string{"INBOX"}, wantRemov: []string{"Label_9"}},
		{name: "out of trash", known: noArchive, source: "TRASH", dest: "INBOX", wantAdd: []string{"INBOX"}, wantRemov: []string{"TRASH"}},
		{name: "never out of sent", known: withArchive, source: "SENT", dest: "Archive", wantAdd: []string{"Label_9"}},
		{name: "unknown source", known: noArchive, source: "Nowhere", dest: "INBOX", wantErr: true},
		{name: "unknown destination", known: noArchive, source: "INBOX", dest: "Nowhere", wantErr: true},
	}
	for _, tt := range tests {
		add, remove, err := moveLabelChange(fakeLabelIDs(tt.known), tt.source, tt.dest)
		if (err != nil) != tt.wantErr {
			t.Fatalf("%s: err = %v, want error %v", tt.name, err, tt.wantErr)
		}
		if !slices.Equal(add, tt.wantAdd) || !slices.Equal(remove, tt.wantRemov) {
			t.Errorf("%s: add %v remove %v, want add %v remove %v", tt.name, add, remove, tt.wantAdd, tt.wantRemov)
		}
	}
}
