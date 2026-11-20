package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sarielhp/feral/calendar"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/tui/render"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *tuiModel) openAttachmentViewer(att email.Attachment) (tea.Model, tea.Cmd) {
	if email.IsICSAttachment(att) {
		return m.openICSViewer(att)
	}

	m.attModalOpen = true
	m.attPreviewAtt = att
	m.attPreviewStatus = ""

	boxW := min(90, max(20, m.width-8))
	if m.width > 0 && boxW > m.width {
		boxW = m.width
	}
	contentW := max(16, boxW-6)
	contentH := min(24, max(5, m.height-10))
	if m.height > 0 && contentH > m.height {
		contentH = m.height
	}

	m.attPreviewVp = viewport.New(contentW, contentH)

	previewText := generateAttachmentPreviewText(att, contentW)
	m.attPreviewVp.SetContent(previewText)

	return m, nil
}

func (m *tuiModel) selectAttachmentByIndex(idx int) (tea.Model, tea.Cmd) {
	if len(m.attachments) == 0 {
		return m, nil
	}
	if idx < 0 {
		idx = len(m.attachments) - 1
	} else if idx >= len(m.attachments) {
		idx = 0
	}
	m.attCursor = idx
	att := m.attachments[idx]
	if email.IsICSAttachment(att) {
		m.attModalOpen = false
		return m.openICSViewer(att)
	}
	m.icsModalOpen = false
	return m.openAttachmentViewer(att)
}

func isPDFAttachment(att email.Attachment) bool {
	ext := strings.ToLower(filepath.Ext(att.Filename))
	ct := strings.ToLower(att.ContentType)
	return ext == ".pdf" || strings.Contains(ct, "pdf") || (len(att.Data) >= 4 && string(att.Data[:4]) == "%PDF")
}

func isTextAttachment(att email.Attachment) bool {
	ct := strings.ToLower(att.ContentType)
	if strings.HasPrefix(ct, "text/") ||
		strings.Contains(ct, "json") ||
		strings.Contains(ct, "xml") ||
		strings.Contains(ct, "javascript") ||
		strings.Contains(ct, "yaml") ||
		strings.Contains(ct, "csv") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(att.Filename))
	switch ext {
	case ".txt", ".md", ".json", ".csv", ".tsv", ".yaml", ".yml",
		".xml", ".html", ".htm", ".css", ".js", ".ts", ".go", ".rs",
		".py", ".rb", ".sh", ".bash", ".c", ".h", ".cpp", ".hpp",
		".diff", ".patch", ".log", ".conf", ".ini", ".env", ".toml":
		return true
	}
	// Check if raw data is valid UTF-8 and contains mostly printable characters
	if len(att.Data) > 0 && len(att.Data) < 2*1024*1024 && utf8.Valid(att.Data) {
		nullCount := 0
		for _, b := range att.Data[:min(len(att.Data), 512)] {
			if b == 0 {
				nullCount++
			}
		}
		if nullCount == 0 {
			return true
		}
	}
	return false
}

func getAttachmentIcon(att email.Attachment) string {
	ext := strings.ToLower(filepath.Ext(att.Filename))
	ct := strings.ToLower(att.ContentType)
	switch {
	case email.IsICSAttachment(att):
		return "⏰ "
	case isPDFAttachment(att):
		return "📕 "
	case ext == ".html" || ext == ".htm" || ct == "text/html":
		return "🌐 "
	case ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" || strings.HasPrefix(ct, "image/"):
		return "🖼️ "
	case ext == ".zip" || ext == ".tar" || ext == ".gz" || ext == ".tgz" || ext == ".7z":
		return "📦 "
	case ext == ".csv" || ext == ".tsv" || ext == ".xlsx" || ext == ".xls":
		return "📊 "
	case ext == ".go" || ext == ".py" || ext == ".rs" || ext == ".js" || ext == ".ts" || ext == ".sh":
		return "⚙️ "
	case isTextAttachment(att):
		return "📄 "
	default:
		return "📎 "
	}
}

func generateAttachmentPreviewText(att email.Attachment, contentW int) string {
	if isTextAttachment(att) {
		return wrapTruncatedLines(string(att.Data), contentW, 1000)
	}

	return renderBinaryAttachmentPreview(att)
}

func wrapTruncatedLines(text string, contentW, maxLines int) string {
	lines := strings.Split(text, "\n")
	var rendered []string
	for i, line := range lines {
		if i >= maxLines {
			rendered = append(rendered, fmt.Sprintf("... (truncated %d more lines)", len(lines)-maxLines))
			break
		}
		rendered = append(rendered, email.WrapText(line, contentW))
	}
	return strings.Join(rendered, "\n")
}

func renderBinaryAttachmentPreview(att email.Attachment) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Binary Attachment: %s\n", att.Filename))
	b.WriteString(fmt.Sprintf("Size: %s (%d bytes)\n", formatFileSize(len(att.Data)), len(att.Data)))
	if att.ContentType != "" {
		b.WriteString(fmt.Sprintf("Content-Type: %s\n", att.ContentType))
	}
	sum := sha256.Sum256(att.Data)
	b.WriteString(fmt.Sprintf("SHA-256: %s\n\n", hex.EncodeToString(sum[:])))

	b.WriteString("Hex Preview (first 128 bytes):\n")
	previewLen := min(128, len(att.Data))
	for i := 0; i < previewLen; i += 16 {
		chunkEnd := min(i+16, previewLen)
		chunk := att.Data[i:chunkEnd]
		hexStr := hex.EncodeToString(chunk)
		var spacedHex strings.Builder
		for j := 0; j < len(hexStr); j += 2 {
			if j > 0 {
				spacedHex.WriteString(" ")
			}
			spacedHex.WriteString(hexStr[j : j+2])
		}
		var asciiStr strings.Builder
		for _, byteVal := range chunk {
			if byteVal >= 32 && byteVal <= 126 {
				asciiStr.WriteByte(byteVal)
			} else {
				asciiStr.WriteByte('.')
			}
		}
		b.WriteString(fmt.Sprintf("%04x: %-48s  |%s|\n", i, spacedHex.String(), asciiStr.String()))
	}

	b.WriteString("\nPress 'o' to open in default system application.")
	return b.String()
}

func formatFileSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	kb := float64(bytes) / 1024.0
	if kb < 1024.0 {
		return fmt.Sprintf("%.1f KB", kb)
	}
	mb := kb / 1024.0
	return fmt.Sprintf("%.1f MB", mb)
}

func (m *tuiModel) kAttachmentModal(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch s {
	case "esc", "q", "f1":
		m.attModalOpen = false
		m.attPreviewStatus = ""
		return m, nil
	case "tab", "n", "]", "right":
		if len(m.attachments) > 1 {
			return m.selectAttachmentByIndex(m.attCursor + 1)
		}
		return m, nil
	case "shift+tab", "p", "[", "left":
		if len(m.attachments) > 1 {
			return m.selectAttachmentByIndex(m.attCursor - 1)
		}
		return m, nil
	case "up", "k":
		m.attPreviewVp.ScrollUp(1)
	case "down", "j":
		m.attPreviewVp.ScrollDown(1)
	case "pgup":
		m.attPreviewVp.HalfPageUp()
	case "pgdown":
		m.attPreviewVp.HalfPageDown()
	case "o", "x", "enter":
		att := m.attPreviewAtt
		tmpFile, err := writeTempAttachment(att)
		if err == nil {
			m.openAttachmentFile(att, tmpFile)
			m.attPreviewStatus = "Opened in external program: " + att.Filename
		} else {
			m.attPreviewStatus = "Error opening: " + err.Error()
		}
		m.attPreviewStatusTime = time.Now()
		return m, nil
	case "s":
		att := m.attPreviewAtt
		savePattern := ""
		if m.Config() != nil {
			savePattern = m.Config().AttachmentSaveDir
		} else if m.cfg != nil {
			savePattern = m.cfg.AttachmentSaveDir
		}

		destDir, err := cfg_g.ExpandSavePath(savePattern, time.Now())
		if err != nil {
			m.attPreviewStatus = "Error resolving path: " + err.Error()
		} else {
			if err := os.MkdirAll(destDir, 0755); err != nil {
				m.attPreviewStatus = "Error creating dir: " + err.Error()
			} else {
				destPath := filepath.Join(destDir, att.Filename)
				if err := os.WriteFile(destPath, att.Data, 0644); err == nil {
					m.attPreviewStatus = "Saved to " + destPath
				} else {
					m.attPreviewStatus = "Error saving: " + err.Error()
				}
			}
		}
		m.attPreviewStatusTime = time.Now()
		return m, nil
	case "S", "b", "B":
		m.openAttachmentSaveDialog(m.attPreviewAtt)
		m.attModalOpen = false
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) syncAttachmentModalViewport() {
	boxW := min(90, max(20, m.width-8))
	if m.width > 0 && boxW > m.width {
		boxW = m.width
	}
	contentW := max(16, boxW-6)
	contentH := min(24, max(5, m.height-10))
	if m.height > 0 && contentH > m.height {
		contentH = m.height
	}
	if m.attPreviewVp.Width != contentW || m.attPreviewVp.Height != contentH {
		m.attPreviewVp.Width = contentW
		m.attPreviewVp.Height = contentH
		if m.attPreviewAtt.Data != nil {
			previewText := generateAttachmentPreviewText(m.attPreviewAtt, contentW)
			m.attPreviewVp.SetContent(previewText)
		}
	}
}

func renderAttachmentModalOverlay(m *tuiModel) string {
	t := m.theme.Theme()
	bg := lipgloss.Color("#1a1a35")
	borderColor := t.Get(render.ColorBorder)
	titleColor := t.Get(render.ColorHighlightFg)

	boxW := min(90, max(20, m.width-8))
	if m.width > 0 && boxW > m.width {
		boxW = m.width
	}
	m.syncAttachmentModalViewport()
	att := m.attPreviewAtt

	titleStyle := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(titleColor)).Bold(true)
	metaStyle := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.Get(render.ColorFg)))
	divStyle := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(borderColor))

	idxInfo := fmt.Sprintf("[%d/%d] ", m.attCursor+1, len(m.attachments))
	title := fmt.Sprintf("%sATTACHMENT PREVIEW: %s (%s)", idxInfo, att.Filename, formatFileSize(len(att.Data)))
	topBar := titleStyle.Render(title)

	metaDivider := divStyle.Render(strings.Repeat("─", max(10, boxW-4)))

	var lines []string
	lines = append(lines, topBar, "")

	if att.ContentType != "" {
		lines = append(lines, metaStyle.Render("MIME: "+att.ContentType))
	}
	lines = append(lines, metaDivider)
	lines = append(lines, m.attPreviewVp.View())
	lines = append(lines, metaDivider)

	dimStyle := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.Get(render.ColorDate)))
	keyStyle := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#ffd700")).Bold(true)

	var footerParts []string
	if len(m.attachments) > 1 {
		footerParts = append(footerParts, keyStyle.Render("[Tab/n] Next"))
	}
	footerParts = append(footerParts, keyStyle.Render("[s] Quick Save"))
	footerParts = append(footerParts, keyStyle.Render("[S] Browse Save"))
	footerParts = append(footerParts, keyStyle.Render("[o] Open External"))
	footerParts = append(footerParts, dimStyle.Render("[Esc/q] Close"))

	lines = append(lines, markHints(fitHints(boxW-4, "  ", footerParts),
		hint{"[Tab/n] Next", "tab"}, hint{"[s] Quick Save", "s"},
		hint{"[S] Browse Save", "S"}, hint{"[o] Open External", "o"}, hint{"[Esc/q] Close", "esc"}))

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Background(bg).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Padding(0, 1).
		Width(boxW).
		Render(content)

	return renderDimmedOverlay(m.width, m.height, m.renderBaseScreen(), box)
}

func renderSelectedAttachmentPreview(m *tuiModel, width int) string {
	if m.attCursor < 0 || m.attCursor >= len(m.attachments) {
		return ""
	}
	att := m.attachments[m.attCursor]
	t := m.theme.Theme()
	borderColor := t.Get(render.ColorBorder)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg)))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorDate)))
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorFg)))

	var lines []string
	switch {
	case isPDFAttachment(att) || isImageAttachment(att):
		lines = append(lines, titleStyle.Render(getAttachmentIcon(att)+" ")+valStyle.Bold(true).Render(att.Filename))
		lines = append(lines, dimStyle.Render(fmt.Sprintf("Size: %s | 'o': open in the OS viewer", formatFileSize(len(att.Data)))))
	case email.IsICSAttachment(att):
		lines = append(lines, titleStyle.Render("⏰ Calendar Event: ")+valStyle.Bold(true).Render(att.Filename))
		if events, err := calendar.ParseAllICSEventsData(att.Data); err == nil && len(events) > 0 {
			ev := events[0]
			lines = append(lines, titleStyle.Render("Title: ")+valStyle.Render(ev.Summary))
			lines = append(lines, titleStyle.Render("When:  ")+valStyle.Render(formatICSEventTime(ev)))
			if ev.ConferenceURL != "" {
				lines = append(lines, titleStyle.Render("Join:  ")+valStyle.Render(ev.ConferenceURL))
			} else if ev.Location != "" {
				lines = append(lines, titleStyle.Render("Loc:   ")+valStyle.Render(ev.Location))
			}
		}
	case isTextAttachment(att):
		lines = append(lines, titleStyle.Render("📄 Text Preview: ")+valStyle.Bold(true).Render(att.Filename))
		lines = append(lines, "")
		rawLines := strings.Split(string(att.Data), "\n")
		for i, rl := range rawLines {
			if i >= 6 {
				break
			}
			wrapped := email.WrapText(rl, width-4)
			lines = append(lines, valStyle.Render(wrapped))
		}
	default:
		lines = append(lines, titleStyle.Render(getAttachmentIcon(att)+" Attachment: ")+valStyle.Bold(true).Render(att.Filename))
		lines = append(lines, valStyle.Render(fmt.Sprintf("Type: %s", att.ContentType)))
		lines = append(lines, valStyle.Render(fmt.Sprintf("Size: %s", formatFileSize(len(att.Data)))))
		lines = append(lines, dimStyle.Render("Press 'o' to open with OS default viewer"))
	}

	content := strings.Join(lines, "\n")
	// Its left edge is the line between it and the list; the drawer draws the
	// rule above both.
	return lipgloss.NewStyle().
		Width(width).
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(true).
		BorderTop(false).
		BorderRight(false).
		BorderBottom(false).
		BorderForeground(lipgloss.Color(borderColor)).
		Padding(0, 1).
		Render(content)
}
