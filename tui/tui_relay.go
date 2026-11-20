package tui

import (
	"context"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/inboxage"
	"github.com/sarielhp/feral/mailclient"
)

// relayCheckInterval is how often the TUI checks its relay accounts -- those
// with inbox_max_age -- for mail left in their inbox. It runs its own check
// rather than relying on the watcher, which may be off (--no-watch) or
// running in another process that holds the watch lock and cannot reach this
// screen.
const relayCheckInterval = 2 * time.Minute

// relayState is the TUI's view of its relay accounts. The client cache is
// touched only by the one check in flight, never by Update.
type relayState struct {
	checking  bool
	lastCheck time.Time
	// alerts is one line per relay that is stale or could not be checked;
	// empty when every relay is drained.
	alerts  []string
	clients map[string]mailclient.MailClient
}

// relayStatusMsg carries one round of relay checks back to Update.
type relayStatusMsg struct {
	at       time.Time
	statuses []inboxage.Status
}

// relayCheckDue reports whether a relay check should start now.
func (m *tuiModel) relayCheckDue(now time.Time) bool {
	if m.relay.checking || m.bk == nil || m.relayConfig() == nil {
		return false
	}
	if len(inboxage.Relays(m.relayConfig().Accounts)) == 0 {
		return false
	}
	return m.relay.lastCheck.IsZero() || now.Sub(m.relay.lastCheck) >= relayCheckInterval
}

// relayConfig is the session's configuration, which lists every account.
func (m *tuiModel) relayConfig() *cfg_g.Config {
	if m.cfg != nil {
		return m.cfg
	}
	return m.Config()
}

// startRelayCheck checks every relay account off the update loop.
func (m *tuiModel) startRelayCheck(now time.Time) tea.Cmd {
	m.relay.checking = true
	m.relay.lastCheck = now
	if m.relay.clients == nil {
		m.relay.clients = map[string]mailclient.MailClient{}
	}
	// A copy, as startTuiWatch takes: the check reads it off the update loop,
	// which may switch accounts meanwhile.
	snapshot := *m.relayConfig()
	snapshot.Accounts = slices.Clone(snapshot.Accounts)
	cfg := &snapshot
	relays := inboxage.Relays(cfg.Accounts)
	clients, provider := m.relay.clients, m.bk.clients()
	return func() tea.Msg {
		out := relayStatusMsg{at: time.Now()}
		for _, acc := range relays {
			out.statuses = append(out.statuses, checkOneRelay(provider, clients, cfg, acc))
		}
		return out
	}
}

// checkOneRelay checks one relay on a client kept between checks, dropping
// the client when the check fails so a renewed sign-in takes effect.
func checkOneRelay(provider ClientProvider, clients map[string]mailclient.MailClient,
	cfg *cfg_g.Config, acc cfg_acc.AccountConfig) inboxage.Status {
	client, ok := clients[acc.Name]
	if !ok {
		c, err := provider.ForAccount(cfg, acc)
		if err != nil {
			return inboxage.Status{Account: acc.Name, Err: err}
		}
		client = c
		clients[acc.Name] = c
	}
	st := inboxage.Check(context.Background(), client, acc, time.Now())
	if st.Err != nil {
		delete(clients, acc.Name)
	}
	return st
}

func (m *tuiModel) handleRelayStatus(msg relayStatusMsg) (tea.Model, tea.Cmd) {
	m.relay.checking = false
	m.relay.alerts = m.relay.alerts[:0]
	for _, st := range msg.statuses {
		if !st.OK() {
			m.relay.alerts = append(m.relay.alerts, st.Summary(msg.at))
		}
	}
	return m, nil
}

// bannerLines is how many rows the banners under the screen take, which every
// layout subtracts from the height it gives the list or the message.
func (m *tuiModel) bannerLines() int {
	n := 0
	if m.showError {
		n++
	}
	if len(m.relay.alerts) > 0 {
		n++
	}
	return n
}

// withBanners appends the relay banner and the error banner to content.
func (m *tuiModel) withBanners(content string) string {
	if len(m.relay.alerts) > 0 {
		content += "\n" + renderRelayBanner(m)
	}
	if m.showError {
		content += "\n" + renderErrorBanner(m)
	}
	return content
}

// renderRelayBanner is one line naming every relay whose inbox is not being
// emptied. It stays until a check finds the relay drained.
func renderRelayBanner(m *tuiModel) string {
	text := "⏳ " + strings.Join(m.relay.alerts, " | ")
	return lipgloss.NewStyle().
		Width(m.width).
		MaxWidth(m.width).
		MaxHeight(1).
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#ffb300")).
		Bold(true).
		Render(text)
}

// withErrorBanner appends the error banner alone, for the screens drawn
// over the list; see banner in screen_layers.go.
func (m *tuiModel) withErrorBanner(content string) string {
	if m.showError {
		return content + "\n" + renderErrorBanner(m)
	}
	return content
}
