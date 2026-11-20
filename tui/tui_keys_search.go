package tui

import (
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// The message list has one filter, typed after "/", "!" or "~". It is live
// while inSearch is set and stays applied once Enter or Tab commits it; Esc
// on a committed filter clears it. "~" also swaps the rows for the whole
// cache, keeping the folder's rows in savedRawEmails until the filter ends.
//
// The message view has its own filter, typed after "/" there, which shows
// only the body lines that match. It belongs to the message on screen, so
// opening another message or leaving the view ends it.

// kSearch owns the keys while the list filter is being typed.
func (m *tuiModel) kSearch(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch {
	case k.Type == tea.KeyEsc:
		m.endListFilter()
	case k.Type == tea.KeyTab || s == "tab" || k.Type == tea.KeyEnter:
		m.inSearch = false
		m.eIdx = 0
		m.selectedID = ""
		m.clampSelection()
	case k.Type == tea.KeyBackspace || s == "backspace" || s == "ctrl+h":
		if m.searchQuery != "" {
			m.setListFilter(dropLastRune(m.searchQuery))
		}
	case k.Type == tea.KeySpace:
		m.setListFilter(m.searchQuery + " ")
	default:
		if len(k.Runes) > 0 {
			m.setListFilter(m.searchQuery + string(k.Runes))
		}
	}
	return m, nil
}

// setListFilter applies query to the list and keeps the selection on a row.
func (m *tuiModel) setListFilter(query string) {
	m.searchQuery = query
	m.rebuildVisibleEmails()
	m.clampSelection()
}

// endListFilter drops the list filter, whether it is being typed or was
// committed, and puts back the folder's own rows when "~" had replaced them
// with the whole cache. It is safe to call with no filter in place.
func (m *tuiModel) endListFilter() {
	active := m.inSearch || m.searchQuery != "" || m.fuzzySearch
	m.inSearch = false
	m.searchQuery = ""
	m.fuzzySearch = false
	if m.fuzzyGlobal && m.savedRawEmails != nil {
		m.rawEmails = m.savedRawEmails
		m.currentF = m.savedCurrentF
	}
	m.fuzzyGlobal = false
	m.savedRawEmails = nil
	m.savedCurrentF = ""
	if !active {
		return
	}
	m.rebuildVisibleEmails()
	m.clampSelection()
}

// clampSelection keeps the selection on a row of the rebuilt list: the
// selected message where it is still shown, the nearest row otherwise.
func (m *tuiModel) clampSelection() {
	if len(m.emails) == 0 {
		m.eIdx = 0
		m.selectedID = ""
		return
	}
	for i := range m.emails {
		if m.selectedID != "" && m.emails[i].ID == m.selectedID {
			m.eIdx = i
			return
		}
	}
	m.eIdx = max(0, min(m.eIdx, len(m.emails)-1))
	m.selectedID = m.emails[m.eIdx].ID
}

// kDetailSearch owns the keys while the message-view filter is being typed.
// It is routed ahead of the global keys, so Esc, Tab, q, f and h reach it
// instead of leaving the message with the prompt still open.
func (m *tuiModel) kDetailSearch(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch {
	case k.Type == tea.KeyEsc:
		m.endDetailFilter()
		return m, nil
	case k.Type == tea.KeyEnter || k.Type == tea.KeyTab || s == "tab":
		m.inDetailSearch = false
		if m.detailSearch == "" {
			m.endDetailFilter()
		}
	case k.Type == tea.KeyBackspace || s == "backspace" || s == "ctrl+h":
		m.detailSearch = dropLastRune(m.detailSearch)
	case k.Type == tea.KeySpace:
		m.detailSearch += " "
	default:
		if len(k.Runes) == 0 {
			return m, nil
		}
		m.detailSearch += string(k.Runes)
	}
	m.detailVpDirty = true
	return m, nil
}

// endDetailFilter closes the message-view filter and shows the whole body.
func (m *tuiModel) endDetailFilter() {
	if !m.inDetailSearch && m.detailSearch == "" {
		return
	}
	m.inDetailSearch = false
	m.detailSearch = ""
	m.detailVpDirty = true
}

// dropLastRune removes the last character of s, not its last byte, so
// Backspace on a Hebrew or accented query leaves valid text.
func dropLastRune(s string) string {
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
}
