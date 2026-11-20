package cli

import (
	"github.com/sarielhp/feral/cache/cachedir"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/email"
)

// storeForExport puts a message in the cache under its own date, as the message
// store does when it downloads one.
func storeForExport(t *testing.T, dir, id string, raw []byte) error {
	t.Helper()
	date := email.ExtractDateOrNow(raw)
	if date.IsZero() {
		date = time.Now()
	}
	return msg.Store(cachedir.At(dir), email.ID(id), raw, date)
}
