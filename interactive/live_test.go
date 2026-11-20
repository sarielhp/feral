package interactive

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/testenv"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/mailops"
)

// The live tests run the TUI in kitty, as the other tests here do, but
// against a real account instead of a fake server: the throwaway test
// account that tools/seed_test_account.rb fills with an Enron-derived
// mailbox (about 1200 messages, folders four levels deep) and a few
// synthetic canaries whose subjects these tests know.
//
// They are read-only. The binary runs with --read-only, so the server is
// never changed, and the account must be marked account_type "test" because
// its token is copied into the private HOME. Nothing else of the user's
// configuration is: the cache, the config file and the browser are the
// harness's own, as for every interactive test.
//
//	FERAL_INTERACTIVE=1 FERAL_LIVE=1 FERAL_LIVE_ACCOUNT=sariel_test \
//	    go test ./interactive/ -run Live -v -count=1

const (
	liveVar        = "FERAL_LIVE"
	liveAccountVar = "FERAL_LIVE_ACCOUNT"

	// serverWait bounds a screen that waits on the real server: the first
	// listing of a folder downloads every summary in it.
	serverWait = 90 * time.Second

	canaryInbox  = "mail_cli canary: inbox plain"
	canaryBody   = "zanzibarquux"
	canaryDeep   = "mail_cli canary: deep folder"
	canaryDeepIn = "Level3"
)

// startLive opens the TUI on the seeded test account, read-only.
func startLive(t *testing.T) *session {
	t.Helper()
	return startLiveWith(t, false)
}

// startLiveWith is startLive, in the program sandbox when sandbox is set.
func startLiveWith(t *testing.T, sandbox bool) *session {
	t.Helper()
	requireInteractive(t)
	if testenv.Getenv(t, liveVar) != "1" {
		t.Skipf("live test; run with %s=1 and %s=<test account>", liveVar, liveAccountVar)
	}
	name := testenv.Getenv(t, liveAccountVar)
	if name == "" {
		t.Skipf("%s must name the seeded test account; there is no default", liveAccountVar)
	}
	config, err := cfg_g.LoadConfigNoWrite()
	if err != nil {
		t.Fatalf("loading the configuration: %v", err)
	}
	acc := liveAccount(t, config, name)
	spec := homeSpec{accounts: []map[string]any{accountMap(t, acc)}, configFiles: tokenFiles(t, config, acc), sandbox: sandbox}
	s := startHome(t, spec, "--read-only", "--account", acc.Name, "tui")
	s.WaitForWithin("Inbox", serverWait)
	return s
}

// liveAccount finds name and refuses anything but a test account: its
// credentials are about to be copied.
func liveAccount(t *testing.T, config *cfg_g.Config, name string) cfg_acc.AccountConfig {
	t.Helper()
	for _, acc := range config.Accounts {
		if !strings.EqualFold(acc.Name, name) {
			continue
		}
		if !strings.EqualFold(acc.AccountType, "test") {
			t.Fatalf("refusing %q: account_type is %q, not \"test\"", name, acc.AccountType)
		}
		return acc
	}
	t.Fatalf("account %q is not configured", name)
	return cfg_acc.AccountConfig{}
}

// accountMap is acc as the config file's accounts list holds it.
func accountMap(t *testing.T, acc cfg_acc.AccountConfig) map[string]any {
	t.Helper()
	data, err := json.Marshal(acc)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// tokenFiles is the account's OAuth token, under the name the binary will
// look for. A refreshed token is written to the private HOME only, so the
// user's copy is never touched.
func tokenFiles(t *testing.T, config *cfg_g.Config, acc cfg_acc.AccountConfig) map[string][]byte {
	t.Helper()
	path, err := mailops.TokenPath(config.ConfigDir, acc)
	if err != nil || path == "" {
		t.Fatalf("no token path for %q (%s): %v", acc.Name, acc.Type, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("account %q has no token; sign in with `feral account login`: %v", acc.Name, err)
	}
	return map[string][]byte{path: data}
}

// typeText types text one character at a time, as a person does; a longer
// string can arrive as a single key event.
func (s *session) typeText(text string) {
	for _, r := range text {
		s.Type(string(r))
	}
}

// waitGone waits until the screen no longer shows unwanted.
func (s *session) waitGone(unwanted string) string {
	s.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		screen := s.Text()
		if !strings.Contains(screen, unwanted) {
			return screen
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("screen still shows %q:\n%s", unwanted, screen)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// staysAbsent fails if unwanted appears on screen within d.
func (s *session) staysAbsent(unwanted string, d time.Duration) {
	s.t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if screen := s.Text(); strings.Contains(screen, unwanted) {
			s.t.Fatalf("screen shows %q:\n%s", unwanted, screen)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// openFolder goes to folder through the folder tree's own search.
func (s *session) openFolder(folder string) {
	s.Key("f")
	s.Type("/")
	s.typeText(folder)
	s.WaitFor(folder)
	s.Key("enter")
}

// openInboxCanary filters the list to the inbox canary and opens it.
func (s *session) openInboxCanary() {
	s.Type("/")
	s.typeText("canary: inbox")
	s.WaitForWithin(canaryInbox, serverWait)
	s.Key("enter") // commit the filter
	s.Key("enter") // open the message
	s.WaitForWithin(canaryBody, serverWait)
}

// A search typed inside a message and closed with Esc used to stay open
// behind the view: Esc reached the global handler first, which left the
// message but not the prompt, and every message opened afterwards showed
// "/query" where its body should be.
func TestLiveMessageSearchEscKeepsBodies(t *testing.T) {
	s := startLive(t)
	s.openInboxCanary()

	s.Type("/")
	s.typeText("zz")
	s.WaitFor("/zz")
	s.Key("escape")
	s.waitGone("/zz")
	s.WaitFor(canaryBody)

	s.Key("escape") // back to the list
	s.Key("escape") // clear the list filter
	s.waitGone("/canary")
	s.Key("enter") // open whatever message is selected
	screen := s.WaitForWithin("From:", serverWait)
	if strings.Contains(screen, "/zz") {
		t.Errorf("the message search came back on another message:\n%s", screen)
	}
}

// q, f and h typed into the message search are text. They used to reach the
// global keys first: q left the message, f opened the folder tree, h help.
func TestLiveMessageSearchTypesGlobalKeys(t *testing.T) {
	s := startLive(t)
	s.openInboxCanary()

	s.Type("/")
	s.typeText("qfh")
	screen := s.WaitFor("/qfh")
	if !strings.Contains(screen, canaryInbox) {
		t.Errorf("typing into the search left the message:\n%s", screen)
	}
}

// The tree is four levels deep on the server; the deepest folder opens, lists
// its message and shows its body.
func TestLiveDeepFolderOpens(t *testing.T) {
	s := startLive(t)
	s.openFolder(canaryDeepIn)
	s.WaitForWithin(canaryDeep, serverWait)
	s.Key("enter")
	s.WaitForWithin("four levels down", serverWait)
}

// "~" searches the whole cache; Esc on the committed filter has to put the
// folder back. It used to clear the query and keep the whole cache as the
// list, so a later search in Inbox found mail from every other folder.
func TestLiveGlobalSearchEscRestoresFolder(t *testing.T) {
	s := startLive(t)
	s.openFolder(canaryDeepIn) // cache the deep folder
	s.WaitForWithin(canaryDeep, serverWait)
	s.Key("escape") // back to Inbox
	s.WaitForWithin("Inbox", serverWait)

	s.Type("~")
	s.typeText("deep folder")
	s.WaitFor(canaryDeep)
	s.Key("enter")
	s.Key("escape")
	s.waitGone("~deep")

	s.Type("/") // the deep canary is not in Inbox
	s.typeText("deep folder")
	s.WaitFor("/deep folder")
	s.staysAbsent(canaryDeep, 2*time.Second)
}

// A Hebrew subject reaches the terminal in visual order in the list.
func TestLiveHebrewSubjectInList(t *testing.T) {
	s := startLive(t)
	s.Type("/")
	s.typeText("mixed direction")
	screen := s.WaitForWithin("mixed direction", serverWait)
	if !strings.Contains(screen, "םלוע םולש") {
		t.Errorf("Hebrew subject not in visual order:\n%s", screen)
	}
}
