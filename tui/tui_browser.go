package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/browser"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/pgp"

	tea "github.com/charmbracelet/bubbletea"
)

type browserOpenedMsg struct {
	id  email.ID
	err error
}

var openBrowserFile = launchBrowserFile

func launchBrowserFile(path string, preferredCmd string) error {
	if preferredCmd != "" {
		if err := browser.OpenWith(preferredCmd, path); err == nil {
			return nil
		}
	}
	return browser.Open(path)
}

func writeTempHTML(content string) (string, error) {
	tempDir, err := app.GetTempDir()
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}
	tempPath := filepath.Join(tempDir, "message.html")
	if err := os.WriteFile(tempPath, []byte(email.ForBrowser(content)), 0600); err != nil {
		return "", fmt.Errorf("failed to write html file: %w", err)
	}
	return tempPath, nil
}

func (m *tuiModel) openCurrentEmailInBrowser() (tea.Model, tea.Cmd) {
	id := m.currentEmailID()
	if id == "" {
		m.saveStatus = "No email selected to view in browser"
		m.saveStatusTime = time.Now()
		return m, nil
	}
	m.saveStatus = "Opening email in browser..."
	m.saveStatusTime = time.Now()
	plan := m.planAction([]email.ID{id}, m.currentF)
	return m, m.cmdOpenInBrowser(id, plan)
}

func (m *tuiModel) cmdOpenInBrowser(id email.ID, plan actionPlan) tea.Cmd {
	preferredBrowser := ""
	if cfg := m.Config(); cfg != nil {
		preferredBrowser = cfg.Browser
	}
	return func() tea.Msg {
		client, err := m.targetClient(plan, id)
		if err != nil {
			return browserOpenedMsg{id: id, err: err}
		}
		data, err := client.GetRawMessage(id)
		if err != nil {
			return browserOpenedMsg{id: id, err: err}
		}
		processed := pgp.Process(context.Background(), data, pgp.Options{Verify: true, Decrypt: true})
		htmlContent := email.HTMLForDisplay(processed.Raw)
		if htmlContent == "" {
			return browserOpenedMsg{id: id, err: fmt.Errorf("message has no displayable body")}
		}
		tempPath, err := writeTempHTML(htmlContent)
		if err != nil {
			return browserOpenedMsg{id: id, err: err}
		}
		if err := openBrowserFile(tempPath, preferredBrowser); err != nil {
			return browserOpenedMsg{id: id, err: err}
		}
		return browserOpenedMsg{id: id, err: nil}
	}
}

// openAttachmentFile opens an attachment written to path: an HTML one in the
// configured browser, as w opens a message, and anything else with the
// desktop's opener. HTML attachments -- a message's own HTML body among
// them -- used to go to the desktop opener too, so with a browser configured
// the same page opened in one browser from w and another from the list.
func (m *tuiModel) openAttachmentFile(att email.Attachment, path string) {
	if !isHTMLAttachment(att) {
		go openSystemFile(path)
		return
	}
	// The page loads nothing remote, as w opens a message (email.ForBrowser).
	if data, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(path, []byte(email.ForBrowser(string(data))), 0o600)
	}
	preferred := ""
	if cfg := m.Config(); cfg != nil {
		preferred = cfg.Browser
	}
	go func() { _ = openBrowserFile(path, preferred) }()
}

func isHTMLAttachment(att email.Attachment) bool {
	ct := strings.ToLower(att.ContentType)
	name := strings.ToLower(att.Filename)
	return strings.HasPrefix(ct, "text/html") || strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".htm")
}
