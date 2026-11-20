package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
)

func (m *tuiModel) startSaveMessage() (tea.Model, tea.Cmd) {
	id := m.currentEmailID()
	if id == "" {
		return m, nil
	}
	if m.lastSaveTargetFolder {
		return m.promptSaveMessageToFolderWithID(id)
	}
	return m.saveCurrentMessage()
}

func (m *tuiModel) promptSaveMessageToFolderWithID(targetID email.ID) (tea.Model, tea.Cmd) {
	if targetID == "" {
		targetID = m.currentEmailID()
	}
	if targetID == "" {
		return m, nil
	}
	src := m.sourceFolderOf(targetID)
	if src == "" {
		m.err = errNoSourceFolder
		m.showError = true
		return m, nil
	}
	m.expandWholeThreadForEmail(targetID)
	m.rebuildVisibleEmails()
	m.savingMessageToFolder = true
	m.savingMessageToDisk = false
	m.savingMessageID = targetID
	m.savingMessageSourceFolder = src
	m.treeOpen = true
	m.showGlobalTree = true
	m.buildEntries()
	return m, nil
}

func (m *tuiModel) startSaveMessageToDisk(id email.ID) (tea.Model, tea.Cmd) {
	if id == "" {
		id = m.currentEmailID()
	}
	if id == "" || m.client == nil {
		return m, nil
	}
	m.savingMessageToDisk = true
	m.savingMessageToFolder = false
	m.savingMessageID = id
	client := m.client
	return m, func() tea.Msg {
		raw, err := client.GetRawMessage(id)
		if err != nil {
			return messageReadyToSaveMsg{err: fmt.Errorf("could not retrieve the message: %w", err)}
		}
		return messageReadyToSaveMsg{stem: messageFileStem(raw), raw: raw}
	}
}

func (m *tuiModel) flipSaveMode() (tea.Model, tea.Cmd) {
	m.setSaveHelp(false)
	targetID := m.savingMessageID
	if targetID == "" {
		targetID = m.currentEmailID()
	}

	if m.savingMessageToFolder {
		m.closeTree()
		m.savingMessageToFolder = false
		m.inFolderSearch = false
		m.folderSearch = ""
		m.lastSaveTargetFolder = false
		return m.startSaveMessageToDisk(targetID)
	}

	if m.savingMessageToDisk {
		m.fileBrowserOpen = false
		m.savingMessageToDisk = false
		m.pendingSaveStem = ""
		m.lastSaveTargetFolder = true
		return m.promptSaveMessageToFolderWithID(targetID)
	}

	return m, nil
}

func (m *tuiModel) kSaveToFolder(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.saveHelpOpen {
		return m.kSaveHelp(k)
	}

	s := k.String()
	switch s {
	case "f1":
		m.setSaveHelp(true)
		return m, nil
	case "f3":
		return m.flipSaveMode()
	case "f2":
		return m.selectFolderEntry()
	case "esc", "q":
		if m.inFolderSearch {
			return m.kFolderSearch(k)
		}
		m.cancelSaveToFolder()
		return m, nil
	}

	if m.inFolderSearch {
		return m.kFolderSearch(k)
	}

	if m.isSaveFolderAllowedKey(s, k) {
		return m.kFolder(k)
	}

	return m, nil
}

// setSaveHelp opens or closes the save dialogs' help, with no search set.
func (m *tuiModel) setSaveHelp(open bool) {
	m.saveHelpOpen = open
	m.helpFind = helpQuery{}
}

// kSaveHelp is the save dialogs' help: "/" searches it and the search takes
// the keys typed into it; any other key closes it, as it always has.
func (m *tuiModel) kSaveHelp(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.String() != "f1" && m.helpFind.key(k) {
		return m, nil
	}
	m.setSaveHelp(false)
	return m, nil
}

func (m *tuiModel) cancelSaveToFolder() {
	m.closeTree()
	m.savingMessageToFolder = false
	m.savingMessageID = ""
	m.savingMessageSourceFolder = ""
	m.inFolderSearch = false
	m.folderSearch = ""
}

func (m *tuiModel) isSaveFolderAllowedKey(s string, k tea.KeyMsg) bool {
	switch {
	case k.Type == tea.KeyUp || s == "k" || k.Type == tea.KeyDown || s == "j":
		return true
	case s == "pageup" || s == "pgup" || s == "pagedown" || s == "pgdown" || s == "pgdn":
		return true
	case s == "home" || s == "end":
		return true
	case k.Type == tea.KeyRight || s == "+" || k.Type == tea.KeyLeft || s == "-":
		return true
	case s == " " || k.Type == tea.KeyTab || s == "tab":
		return true
	case k.Type == tea.KeyEnter || s == "/":
		return true
	default:
		return false
	}
}

func (m *tuiModel) kSaveToDisk(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.saveHelpOpen {
		return m.kSaveHelp(k)
	}

	s := k.String()
	switch s {
	case "f1":
		m.setSaveHelp(true)
		return m, nil
	case "f3":
		return m.flipSaveMode()
	case "esc", "q":
		if m.fileBrowser.InSearch || m.fileBrowser.EnteringPath || m.fileBrowser.CreatingDir {
			newFb, cmd := m.fileBrowser.Update(k)
			return m.afterFileBrowser(newFb, cmd)
		}
		m.cancelSaveToDisk()
		return m, nil
	}

	if m.fileBrowser.InSearch || m.fileBrowser.EnteringPath || m.fileBrowser.CreatingDir {
		newFb, cmd := m.fileBrowser.Update(k)
		return m.afterFileBrowser(newFb, cmd)
	}

	if m.isSaveDiskAllowedKey(s) {
		newFb, cmd := m.fileBrowser.Update(k)
		return m.afterFileBrowser(newFb, cmd)
	}

	return m, nil
}

func (m *tuiModel) cancelSaveToDisk() {
	m.fileBrowserOpen = false
	m.savingMessageToDisk = false
	m.pendingSaveStem = ""
	m.savingMessageID = ""
}

func (m *tuiModel) isSaveDiskAllowedKey(s string) bool {
	switch s {
	case "up", "k", "down", "j", "pageup", "pgup", "pagedown", "pgdn", "pgdown", "home", "end":
		return true
	case "enter", "right", "l", " ", "backspace", "left", "h":
		return true
	case "s", "S", "f2":
		return true
	case "n", "N", "/", ".", "H", ":", "g", "~":
		return true
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		return true
	default:
		return false
	}
}

func renderSaveHelp(m *tuiModel) string {
	dialogWidth := min(72, max(48, m.width-4))
	inner := dialogWidth - 4

	var title string
	var rows []helpRow
	if m.savingMessageToFolder {
		title = "ACTIVE KEYS - SAVE TO LABEL"
		rows = []helpRow{
			{keys: "Enter / F2", desc: "Save to selected folder/label"},
			{keys: "F3", desc: "Switch to Save to Disk view"},
			{keys: "↑ / k, ↓ / j", desc: "Navigate folder tree"},
			{keys: "PgUp / PgDn", desc: "Page up / down"},
			{keys: "Home / End", desc: "First / last folder"},
			{keys: "→ / +, ← / -", desc: "Expand / collapse subtree"},
			{keys: "Space, Tab", desc: "Toggle / cycle expansion"},
			{keys: "/", desc: "Search folder names"},
			{keys: "Esc / q", desc: "Cancel and return"},
		}
	} else {
		title = "ACTIVE KEYS - SAVE TO DISK"
		rows = []helpRow{
			{keys: "Enter / F2 / s", desc: "Save file in current directory"},
			{keys: "F3", desc: "Switch to Save to Label view"},
			{keys: "↑ / k, ↓ / j", desc: "Navigate entries"},
			{keys: "Enter / → / l", desc: "Open directory"},
			{keys: "Backspace / ←", desc: "Parent directory"},
			{keys: "1 - 9", desc: "Jump to saved location"},
			{keys: "~", desc: "Jump to home directory"},
			{keys: "/", desc: "Filter directory contents"},
			{keys: "n", desc: "Create new directory"},
			{keys: ": / g", desc: "Enter path directly"},
			{keys: "Esc / q", desc: "Cancel and return"},
		}
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffd700")).
		Width(inner).Align(lipgloss.Center).Render(title))
	sb.WriteString("\n\n")

	shown := m.helpFind.filter(rows)
	for _, r := range shown {
		keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55ff55")).Width(16)
		descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#dddddd"))
		sb.WriteString("  " + keyStyle.Render(r.keys) + descStyle.Render(r.desc) + "\n")
	}
	if len(shown) == 0 {
		sb.WriteString(noHelpMatch + "\n")
	}

	sb.WriteString("\n")
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).
		Render(truncateTo(smallHelpFooter(m.helpFind, "Press F1, Esc, or Enter to close"), inner))
	sb.WriteString(footer + "\n")

	dialogBox := lipgloss.NewStyle().
		Background(lipgloss.Color("#16162a")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#ffd700")).
		Padding(1, 2).
		Width(dialogWidth).
		Render(sb.String())

	return renderDimmedOverlay(m.width, m.height, m.renderBaseScreen(), dialogBox)
}

func saveFolderBoxDimensions(m *tuiModel) (int, int) {
	boxW := min(86, max(20, m.width-4))
	if m.width > 0 && boxW > m.width {
		boxW = m.width
	}
	boxH := min(24, max(5, m.height-6))
	if m.height > 0 && boxH > m.height {
		boxH = m.height
	}
	return boxW, boxH
}

func (m *tuiModel) saveFolderInfoLines(boxW int, headerStyle, pathStyle lipgloss.Style) []string {
	var lines []string
	lines = append(lines, headerStyle.Render(" 📁 SELECT SAVE LABEL / FOLDER "))
	lines = append(lines, "")

	msgLabel := string(m.savingMessageID)
	if em := m.findEmailByID(m.savingMessageID); em != nil && em.Subject != "" {
		msgLabel = em.Subject
	}
	maxLabelW := boxW - 18
	if len([]rune(msgLabel)) > maxLabelW && maxLabelW > 3 {
		msgLabel = string([]rune(msgLabel)[:maxLabelW-3]) + "..."
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#aaaaaa")).Render(" Saving Message: "+msgLabel))

	currentFolder := ""
	if m.cursor >= 0 && m.cursor < len(m.entries) && m.entries[m.cursor].Node != nil {
		currentFolder = m.entries[m.cursor].Node.FullName
	}
	if currentFolder == "" {
		currentFolder = m.currentF
	}
	maxFolderW := boxW - 18
	if len([]rune(currentFolder)) > maxFolderW && maxFolderW > 3 {
		currentFolder = string([]rune(currentFolder)[:maxFolderW-3]) + "..."
	}
	lines = append(lines, pathStyle.Render(" Target Folder:  "+currentFolder))
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#334466")).Render(strings.Repeat("─", max(10, boxW-4))))

	if m.inFolderSearch {
		prompt := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffdd44")).Render(" Filter: /")
		val := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Render(m.folderSearch + "█")
		lines = append(lines, prompt+val)
	}
	return lines
}

func renderFolderEntryRow(e label.TreeEntry, isSel bool, boxW int) string {
	prefix := "   "
	if isSel {
		prefix = " > "
	}
	var arrow string
	if e.Node != nil && len(e.Node.Children) > 0 {
		if e.Node.Expanded {
			arrow = "[-] "
		} else {
			arrow = "[+] "
		}
	}
	icon := "📁 "
	if e.Node != nil && len(e.Node.Children) > 0 && e.Node.Expanded {
		icon = "📂 "
	}

	treeIndent := e.Prefix + arrow
	folderName := e.Text
	if e.Node != nil && e.Node.MessagesUnread > 0 {
		folderName += fmt.Sprintf(" (%d)", e.Node.MessagesUnread)
	}

	maxNameW := boxW - 8
	if maxNameW < 10 {
		maxNameW = 10
	}
	avail := max(4, maxNameW-len([]rune(prefix))-len([]rune(treeIndent))-len([]rune(icon)))
	if len([]rune(folderName)) > avail && avail > 3 {
		folderName = string([]rune(folderName)[:avail-3]) + "..."
	}

	if isSel {
		style := lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("#333366")).Foreground(lipgloss.Color("#ffffff"))
		line := fmt.Sprintf("%s%s%s%-*s", prefix, treeIndent, icon, avail, folderName)
		return style.Render(line)
	}
	treeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#556688"))
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#66ccff"))
	return fmt.Sprintf("%s%s%s%s", prefix, treeStyle.Render(treeIndent), icon, nameStyle.Render(folderName))
}

func (m *tuiModel) saveFolderListingLines(availLines, boxW int) []string {
	if len(m.entries) == 0 {
		return []string{lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("   (no folders available)")}
	}
	start := m.cursor - availLines/2
	if start < 0 {
		start = 0
	}
	end := start + availLines
	if end > len(m.entries) {
		end = len(m.entries)
		start = end - availLines
		if start < 0 {
			start = 0
		}
	}
	var lines []string
	for i := start; i < end; i++ {
		lineStr := renderFolderEntryRow(m.entries[i], i == m.cursor, boxW)
		lines = append(lines, treeZone(i, lineStr))
	}
	return lines
}

func renderSaveFolderDialog(m *tuiModel) string {
	boxW, boxH := saveFolderBoxDimensions(m)
	bg := lipgloss.Color("#16162a")
	borderC := lipgloss.Color("#55ffff")

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(borderC).Background(bg)
	pathStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffd700")).Background(bg)

	lines := m.saveFolderInfoLines(boxW, headerStyle, pathStyle)

	footerText := " [Enter/F2] Save to Label | [F3] Save to Disk | [F1] Keys\n [/] Filter | [Tab] Fold | [j/k, ↑/↓] Move | [Esc] Cancel"
	footerRows := strings.Count(footerText, "\n") + 1
	availLines := max(3, boxH-len(lines)-3-footerRows)

	lines = append(lines, m.saveFolderListingLines(availLines, boxW)...)

	for len(lines) < boxH-2-footerRows {
		lines = append(lines, "")
	}

	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#334466")).Render(strings.Repeat("─", max(10, boxW-4))))
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#8888aa")).Render(footerText))

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Background(bg).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(borderC).
		Padding(0, 1).
		Width(boxW).
		Render(content)
}
