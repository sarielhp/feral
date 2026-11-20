package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/cache/virtual"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

// searchFolderPrefix names the TUI's view of remote search results. It is not
// a server folder.
const searchFolderPrefix = "%search:"

func isSearchView(folder string) bool { return strings.HasPrefix(folder, searchFolderPrefix) }

// errSearchViewAction refuses a move out of a remote search view. A search hit
// can sit in any folder and the server's summaries do not say which, while
// every move names a source folder; "%search:<q>" used to be sent as that
// source, which on Gmail is a label removal that matches nothing.
var errSearchViewAction = errors.New("remote search results do not record each message's folder; open its folder to move, delete or report it")

// remoteSearchPageSize is how many of the newest matches a Ctrl+f search
// fetches. Every hit costs a summary download, so asking for all of them made a
// broad query crawl; + in the results view doubles the count. It is not R,
// which refreshes everywhere and sits beside r (reply).
const remoteSearchPageSize = 25

type remoteSearchDoneMsg struct {
	query  string
	emails []*email.Email
	err    error
	// client is the one the search ran against, captured when it started.
	client mailclient.MailClient
	// limit is what the server was asked for; more reports that it filled
	// the limit, so further matches may exist.
	limit int
	more  bool
}

func (m *tuiModel) kRemoteSearchPrompt(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	q, ok := m.kPromptLine(k, &m.remoteSearchQuery, &m.inRemoteSearch)
	if !ok {
		return m, nil
	}
	return m.startRemoteSearch(q, remoteSearchPageSize)
}

// startRemoteSearch queries the server and shows the hits as the search
// view. A limit of zero or less means every match; a positive one keeps the
// query to the newest few, as the alias entry's latest-mail action wants.
func (m *tuiModel) startRemoteSearch(query string, limit int) (tea.Model, tea.Cmd) {
	if m.client == nil {
		m.err = fmt.Errorf("no mail client connected for remote search")
		m.showError = true
		return m, nil
	}

	m.showLoad = "Searching server..."
	client := m.client

	return m, func() tea.Msg {
		done := remoteSearchDoneMsg{query: query, client: client, limit: limit}
		ids, err := client.SearchEmails(query, limit)
		if err != nil {
			done.err = err
			return done
		}
		done.more = limit > 0 && len(ids) >= limit
		if len(ids) == 0 {
			done.emails = []*email.Email{}
			return done
		}
		// Summaries cached by an earlier, smaller run are not downloaded
		// again, so loading more pays only for the new hits.
		done.emails, done.err = client.GetMessageSummariesBatch(ids)
		return done
	}
}

// loadMoreSearchResults re-runs the search view's query with twice the limit.
// Re-running is simpler than holding a server page token across key presses,
// and costs one ID listing plus the summaries not yet cached.
func (m *tuiModel) loadMoreSearchResults() (tea.Model, tea.Cmd) {
	if !m.remoteSearchMore {
		m.saveStatus = fmt.Sprintf("All %d match(es) are shown", len(m.rawEmails))
		m.saveStatusTime = time.Now()
		return m, nil
	}
	query := strings.TrimPrefix(m.currentF, searchFolderPrefix)
	return m.startRemoteSearch(query, max(m.remoteSearchLimit, remoteSearchPageSize)*2)
}

// refreshSearchResults re-runs the search view's query at the limit it
// was fetched with: R refreshes a view, and in the search view that is the
// same page of newest matches, brought up to date.
func (m *tuiModel) refreshSearchResults() (tea.Model, tea.Cmd) {
	query := strings.TrimPrefix(m.currentF, searchFolderPrefix)
	return m.startRemoteSearch(query, max(m.remoteSearchLimit, remoteSearchPageSize))
}

// sameAccount compares two clients by the account they serve, never by
// pointer: the decorators are built per call.
func sameAccount(a, b mailclient.MailClient) bool {
	if a == nil || b == nil || a.Config() == nil || b.Config() == nil {
		return false
	}
	return strings.EqualFold(string(a.Config().SelectedAccount), string(b.Config().SelectedAccount))
}

func (m *tuiModel) handleRemoteSearchDone(msg remoteSearchDoneMsg) (tea.Model, tea.Cmd) {
	m.showLoad = ""
	if msg.err != nil {
		m.err = fmt.Errorf("remote search failed: %w", msg.err)
		m.showError = true
		return m, nil
	}
	searched := msg.client
	if searched == nil {
		searched = m.client
	}

	rawList := make([]email.Email, 0, len(msg.emails))
	ids := make([]email.ID, 0, len(msg.emails))
	for _, em := range msg.emails {
		if em != nil {
			rawList = append(rawList, *em)
			ids = append(ids, em.ID)
		}
	}

	// The result set belongs to the account searched, which is not the
	// active one if the user switched while the search ran.
	if cfg := searched.Config(); cfg != nil && !cfg.AccountCache.IsZero() {
		vm := &virtual.Mailbox{
			Name:        virtual.NameForQuery(msg.query),
			Description: fmt.Sprintf("Results for %q", msg.query),
			MessageIDs:  ids,
		}
		_ = virtual.Save(cfg.AccountCache, vm)
	}
	if msg.client != nil && !sameAccount(msg.client, m.client) {
		m.saveStatus = fmt.Sprintf("Search %q finished on %s (%d result(s)); switch back to view it", msg.query, accountLabel(searched), len(rawList))
		m.saveStatusTime = time.Now()
		return m, nil
	}

	view := searchFolderPrefix + msg.query
	var keep email.ID
	if m.currentF == view {
		keep = m.selectedID // loading more: stay on the same message
	}
	m.endListFilter()
	m.pushHistory(view)
	m.rawEmails = rawList
	m.currentF = view
	m.remoteSearchLimit = msg.limit
	m.remoteSearchMore = msg.more
	m.rebuildVisibleEmails()
	m.selectSearchRow(keep)

	m.saveStatus = fmt.Sprintf("Found %d remote message(s)", len(rawList))
	if msg.more {
		m.saveStatus += " (newest only; + loads more)"
	}
	m.saveStatusTime = time.Now()
	return m, nil
}

// selectSearchRow puts the cursor on the message with the given ID, or on the
// first row when it is empty or no longer listed.
func (m *tuiModel) selectSearchRow(id email.ID) {
	m.eIdx = 0
	m.selectedID = ""
	for i := range m.emails {
		if m.emails[i].ID == id {
			m.eIdx = i
			break
		}
	}
	if len(m.emails) > 0 {
		m.selectedID = m.emails[m.eIdx].ID
	}
}
