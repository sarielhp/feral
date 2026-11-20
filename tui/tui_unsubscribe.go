package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// X leaves the mailing list the current message came from. The plan comes
// from email.PrepareUnsubscribe, the same as `msg unsub`; this file only
// asks and carries it out. Under read-only or a dry run the dialog shows the
// plan and y reports it, sending nothing.

// unsubscribePost is the one-click POST. The state-machine walk presses y in
// the dialog, so it replaces this: a walk must not reach the network.
var unsubscribePost = email.ExecuteOneClickUnsubscribe

// unsubPrompt is the confirmation on screen: what would be done, and the
// account that would do it.
type unsubPrompt struct {
	unsub    email.Unsubscription
	client   mailclient.MailClient
	account  string
	simulate bool
}

type unsubPreparedMsg struct {
	id     email.ID
	dialog *unsubPrompt
	err    error
}

type unsubDoneMsg struct {
	unsub email.Unsubscription
	err   error
}

// promptUnsubscribe reads the current message's headers in the background
// and opens the confirmation on what they say.
func (m *tuiModel) promptUnsubscribe() (tea.Model, tea.Cmd) {
	id := m.currentEmailID()
	if id == "" {
		return m, nil
	}
	client, err := m.targetClient(m.planAction([]email.ID{id}, m.currentF), id)
	if err != nil {
		m.err = fmt.Errorf("unsubscribe: %w", err)
		m.showError = true
		return m, nil
	}
	simulate := m.isAccountReadOnly() || m.isDryRun() || mailclient.Refuses(client)
	m.saveStatus = "Reading the unsubscribe header..."
	m.saveStatusTime = time.Now()
	return m, func() tea.Msg {
		raw, err := client.GetRawMessage(id)
		if err != nil {
			return unsubPreparedMsg{id: id, err: err}
		}
		unsub, err := email.PrepareUnsubscribe(raw)
		if err != nil {
			return unsubPreparedMsg{id: id, err: err}
		}
		from, account := clientFromHeader(client)
		unsub.From = from
		return unsubPreparedMsg{id: id, dialog: &unsubPrompt{unsub: unsub, client: client, account: account, simulate: simulate}}
	}
}

// clientFromHeader is the From header of the account client belongs to, and
// that account's name.
func clientFromHeader(client mailclient.MailClient) (string, string) {
	cfg := client.Config()
	if cfg == nil {
		return "", ""
	}
	for _, acc := range cfg.Accounts {
		if cfg.IsSelected(acc) {
			return formatFromHeader(acc, cfg), acc.Name
		}
	}
	return "", ""
}

// handleUnsubMsg takes the two messages the unsubscribe commands send.
func (m *tuiModel) handleUnsubMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case unsubPreparedMsg:
		return m.handleUnsubPrepared(msg)
	case unsubDoneMsg:
		return m.handleUnsubDone(msg)
	}
	return m, nil
}

// handleUnsubPrepared opens the dialog on what the header said -- but only
// while the list or the message is in front and still on the message X was
// pressed on: a body that took a while to download must not open a dialog
// behind compose or help, nor ask about a message the user has left.
func (m *tuiModel) handleUnsubPrepared(msg unsubPreparedMsg) (tea.Model, tea.Cmd) {
	m.saveStatusTime = time.Now()
	front := m.keyScreen()
	switch {
	case (front != scrList && front != scrMessage) || m.currentEmailID() != msg.id:
		m.saveStatus = "Unsubscribe dropped: the screen changed before the header was read."
	case errors.Is(msg.err, email.ErrNoListUnsubscribe):
		m.saveStatus = "No List-Unsubscribe header: this message names no list to leave."
	case msg.err != nil:
		m.err = fmt.Errorf("unsubscribe: %w", msg.err)
		m.showError = true
	default:
		m.saveStatus = ""
		m.unsubDialog = msg.dialog
	}
	return m, nil
}

// kUnsubscribe answers the dialog: y carries the plan out, anything that
// reads as no closes it, and every other key is swallowed.
func (m *tuiModel) kUnsubscribe(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "y", "Y":
		return m.confirmUnsubscribe()
	case "esc", "n", "N", "q", "Q", "c", "C":
		m.unsubDialog = nil
		m.saveStatus = "Unsubscribe cancelled."
		m.saveStatusTime = time.Now()
	}
	return m, nil
}

func (m *tuiModel) confirmUnsubscribe() (tea.Model, tea.Cmd) {
	d := m.unsubDialog
	m.unsubDialog = nil
	m.saveStatusTime = time.Now()
	if d == nil {
		return m, nil
	}
	if d.simulate {
		m.saveStatus = "[RO] Would " + d.unsub.Summary() + "; nothing was sent."
		return m, nil
	}
	cfg := d.client.Config()
	configDir, browserCmd := "", ""
	if cfg != nil {
		configDir, browserCmd = cfg.ConfigDir, cfg.Browser
	}
	fx := email.UnsubscribeEffects{
		PostOneClick: unsubscribePost,
		OpenBrowser:  func(u string) error { return openBrowserFile(u, browserCmd) },
		SendMail:     d.client.SendEmail,
	}
	m.saveStatus = "Unsubscribing..."
	return m, func() tea.Msg {
		return unsubDoneMsg{unsub: d.unsub, err: d.unsub.CarryOut(fx, configDir)}
	}
}

func (m *tuiModel) handleUnsubDone(msg unsubDoneMsg) (tea.Model, tea.Cmd) {
	m.saveStatusTime = time.Now()
	switch {
	case msg.err != nil:
		m.err = fmt.Errorf("unsubscribing from %s failed: %w", msg.unsub.Sender, msg.err)
		m.showError = true
	case msg.unsub.Plan.Method == email.UnsubscribeBrowser:
		m.saveStatus = "Opened the unsubscribe page in the browser; finish there."
	default:
		m.saveStatus = fmt.Sprintf("Unsubscribed from %s (%s).", msg.unsub.Sender, msg.unsub.Plan.Method)
	}
	return m, nil
}

// unsubPreviewLines caps the preview drawn in the dialog: a mailto body is
// the sender's text and may run long. The plan comes first, so the cap never
// hides the address or link being confirmed.
const unsubPreviewLines = 16

func renderUnsubscribeDialog(m *tuiModel) string {
	d := m.unsubDialog
	width := min(80, max(24, m.width-4))
	lines := wrapUnsubPreview(d.unsub.Preview(), max(10, width-6))

	var content strings.Builder
	content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffaa00")).Render("UNSUBSCRIBE") + "\n\n")
	content.WriteString(strings.Join(lines, "\n") + "\n\n")
	if d.account != "" {
		content.WriteString("  Account: " + d.account + "\n")
	}
	if d.simulate {
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Render("  [RO] Read-only: y only reports; nothing is sent.") + "\n")
	}
	content.WriteString("\n" +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaa00")).Bold(true).Render("  [y] Unsubscribe") + "   " +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(markHints("[n/Esc] Cancel", hint{"[n/Esc] Cancel", "esc"})))

	box := lipgloss.NewStyle().
		Background(lipgloss.Color("#16162a")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#ffaa00")).
		Padding(1, 2).
		Width(width).
		Render(content.String())
	return renderDimmedOverlay(m.width, m.height, m.renderBaseScreen(), box)
}

// wrapUnsubPreview breaks the preview into lines of at most width runes. A
// link is broken rather than cut: what is confirmed is shown whole.
func wrapUnsubPreview(preview string, width int) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimRight(preview, "\n"), "\n") {
		r := []rune(line)
		for len(r) > width {
			out = append(out, string(r[:width]))
			r = append([]rune("    "), r[width:]...)
		}
		out = append(out, string(r))
	}
	if len(out) > unsubPreviewLines {
		out = append(out[:unsubPreviewLines], "  …")
	}
	return out
}
