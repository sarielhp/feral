package cfg_g

// The configuration's shared state -- what a later `cfg sync` will export to
// another machine -- is config.json and, for each account, the four files
// under accounts/<name>/: account.json, whitelist.json, blacklist.json and
// rules.json. Every write of one of them ends in writeShared, so that a step
// every such write needs (the forbidden-write check, the file's canonical
// bytes, and later the export) is made in one place and cannot be missed by
// a new writer. TestSharedStateHasOneWriter keeps new direct writes out.
//
// Moves are not writes and do not pass through here: renaming an account
// moves its directory (runner's account rename, MigrateDirSanitization), and
// the configuration directory itself moves once (MigrateConfigDir). The
// contacts book, whose aliases are shared too, has its own single writer,
// contacts.SaveBook, reached only through contacts.Update.

// SharedFiles are the names of the files that hold shared state: config.json
// in the configuration directory, the rest in each account's directory.
var SharedFiles = []string{"config.json", "account.json", "whitelist.json", "blacklist.json", "rules.json"}

// writeShared writes data, a JSON document, to path, one of the shared
// files, in the canonical layout (canonicalJSON). It refuses under
// --dry-run, like SaveConfigFile, so a writer that does not go through
// SaveConfigFile (an account rename's in-place edit) is covered as well.
// It returns the bytes written.
func writeShared(path string, data []byte) ([]byte, error) {
	if configWritesForbidden.Load() {
		return nil, ErrConfigWriteForbidden
	}
	canonical, err := canonicalJSON(path, data)
	if err != nil {
		return nil, err
	}
	return canonical, atomicWriteFile(path, canonical, 0o600)
}
