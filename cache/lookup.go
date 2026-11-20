package cache

import (
	"encoding/json"
	"fmt"
	"github.com/sarielhp/feral/cache/cachedir"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/email"
)

// FindCachedEmailByID resolves ref -- a message ID as the user typed it, full
// or short -- to a cached message, and names the folder index holding it.
func FindCachedEmailByID(account cachedir.Account, ref string) (email.ID, string, error) {
	downloadDir := account.Path()
	foundID, err := resolveCachedRef(account, strings.TrimSpace(ref))
	if err != nil {
		return "", "", err
	}

	idxDir := filepath.Join(downloadDir, "indexes")
	entries, err := os.ReadDir(idxDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			data, rErr := os.ReadFile(filepath.Join(idxDir, e.Name()))
			if rErr != nil {
				continue
			}
			var ids []email.ID
			if err := json.Unmarshal(data, &ids); err != nil {
				continue
			}
			for _, id := range ids {
				if id == foundID {
					return foundID, strings.TrimSuffix(e.Name(), ".json"), nil
				}
			}
		}
	}

	return foundID, "", nil
}

// resolveCachedRef finds the one cached message ref names.
//
// A full ID wins outright, so a short ID that collides with some other
// message's never shadows an exact match. Short IDs are 32-bit hashes and can
// collide; when two cached messages share one, ref is refused as ambiguous
// rather than resolved to whichever the map iteration happened to reach first,
// which differed from run to run.
func resolveCachedRef(account cachedir.Account, ref string) (email.ID, error) {
	var exact email.ID
	var short []email.ID
	msg.ForEachID(account, func(id email.ID) bool {
		if strings.EqualFold(string(id), ref) {
			exact = id
			return true
		}
		if email.ComputeShortID(id).Matches(ref) {
			short = append(short, id)
		}
		return false
	})
	if exact != "" {
		return exact, nil
	}
	switch len(short) {
	case 0:
		return "", fmt.Errorf("no cached email found with ID %q", ref)
	case 1:
		return short[0], nil
	}
	names := make([]string, len(short))
	for i, id := range short {
		names[i] = string(id)
	}
	sort.Strings(names)
	return "", fmt.Errorf("short ID %q is ambiguous; it matches %s -- use the full ID", ref, strings.Join(names, ", "))
}
