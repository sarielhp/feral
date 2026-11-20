package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/sarielhp/feral/email"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *tuiModel) renderComposeScreen() string {
	if m.compose == nil {
		return ""
	}

	// A send waits on the server; say so, or the screen looks frozen.
	if m.compose.sending {
		return renderSendingDialog(m.width, m.height, m.compose)
	}

	if m.compose.DiscardConfirm {
		return renderDiscardConfirmDialog(m)
	}

	if m.compose.WarnEmptySend {
		return renderWarnEmptySendDialog(m)
	}

	if m.compose.SendConfirm {
		return renderSendConfirmDialog(m)
	}

	bg := lipgloss.Color("#16162a")
	// The screen is the whole terminal, with no frame: a border along the
	// terminal's own edge separates nothing, takes two rows and four columns,
	// and lands in every Shift-drag selection. The rules between the headers,
	// the body and the footer stay, since they do separate something.
	boxW := max(40, m.width)

	var lines []string

	// 1. Top Header Banner
	accLabel := m.compose.AccountName
	if accLabel == "" {
		accLabel = "default"
	}
	topTitle := fmt.Sprintf(" ✉ COMPOSE & SEND REVIEW  [%s] ", accLabel)
	if badge := m.composePgpBadge(); badge != "" {
		topTitle += " " + badge + " "
	}
	if m.mouseOff {
		topTitle += " " + mouseOffHint + " "
	}
	topStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#ffd700")).
		Background(lipgloss.Color("#2a2a4e")).
		Padding(0, 1)
	lines = append(lines, topStyle.Render(topTitle), "")

	// 2. Header Fields
	lines = append(lines, renderComposeHeaderFields(m)...)
	lines = append(lines, "")

	// 3. Attachments List
	lines = append(lines, renderComposeAttachments(m)...)
	lines = append(lines, "")

	// 4. Body Viewport
	divStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#445588"))
	divider := divStyle.Render(strings.Repeat("─", max(10, boxW-2)))
	bodyHeader := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55ffff")).Render(" Message Body:")
	lines = append(lines, divider, bodyHeader)

	// 5. Footer Menu / Keybindings
	footer := renderComposeFooter(boxW)
	footerLines := strings.Count(footer, "\n") + 1

	// Below the body: a divider and the footer rows.
	usedLines := len(lines) + 1 + footerLines
	availHeight := max(4, m.height-usedLines)
	if m.compose.Vp.Height != availHeight || m.compose.Vp.Width != boxW-2 {
		m.compose.Vp.Height = availHeight
		m.compose.Vp.Width = boxW - 2
		m.compose.Vp.SetContent(m.compose.Body)
	}
	lines = append(lines, keyZone("e", zoneRow, m.compose.Vp.View()), divider, footer)

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Background(bg).
		Padding(0, 1).
		Width(boxW).
		Height(m.height).
		Render(content)
}

func renderComposeHeaderFields(m *tuiModel) []string {
	var lines []string
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffdd44"))
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff"))
	editStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55ffff")).Background(lipgloss.Color("#333366"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	// From
	fromVal := m.compose.From
	if fromVal == "" {
		fromVal = "(none)"
	}
	lines = append(lines, fmt.Sprintf(" %-9s %s %s", labelStyle.Render("From:"), valStyle.Render(fromVal), dimStyle.Render("(F3 to change)")))

	// To, Cc and Bcc are the same row behind a different field.
	addrRows := []struct {
		label string
		field composeField
		value string
		hint  string
	}{
		{"To:", fieldTo, m.compose.To, "(none — press 't' to edit)"},
		{"Cc:", fieldCc, m.compose.Cc, "(none — press 'c' to edit)"},
		{"Bcc:", fieldBcc, m.compose.Bcc, "(none — press 'b' to edit)"},
	}
	for _, row := range addrRows {
		if m.compose.FieldEditing == row.field {
			lines = append(lines, fmt.Sprintf(" %-9s %s %s", labelStyle.Render(row.label), editStyle.Render(m.compose.FieldInput+"█"), dimStyle.Render("[Enter: Save, Tab: Complete, F5: Contacts, Esc: Cancel]")))
			continue
		}
		val := row.value
		if val == "" {
			val = dimStyle.Render(row.hint)
		} else {
			val = valStyle.Render(val)
		}
		lines = append(lines, fmt.Sprintf(" %-9s %s", labelStyle.Render(row.label), val))
	}

	// Subject
	if m.compose.FieldEditing == fieldSubject {
		lines = append(lines, fmt.Sprintf(" %-9s %s %s", labelStyle.Render("Subject:"), editStyle.Render(m.compose.FieldInput+"█"), dimStyle.Render("[Enter: Save, Esc: Cancel]")))
	} else {
		subjVal := m.compose.Subject
		if subjVal == "" {
			if m.compose.suggestingSubject {
				subjVal = dimStyle.Render("(generating subject with LLM...)")
			} else {
				subjVal = dimStyle.Render("(no subject — press 's' to edit, F4 for AI)")
			}
		} else {
			subjVal = valStyle.Bold(true).Render(subjVal)
		}
		lines = append(lines, fmt.Sprintf(" %-9s %s", labelStyle.Render("Subject:"), subjVal))
	}

	// Each row is clickable as the key that edits it.
	for i, k := range []string{"f3", "t", "c", "b", "s"} {
		lines[i] = keyZone(k, zoneRow, lines[i])
	}
	return lines
}

func renderComposeAttachments(m *tuiModel) []string {
	var lines []string
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffdd44"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	if len(m.compose.Attachments) == 0 {
		lines = append(lines, keyZone("a", zoneRow, fmt.Sprintf(" %-9s %s", labelStyle.Render("Attach:"), dimStyle.Render("(none — press 'a' to attach a file)"))))
		return lines
	}

	lines = append(lines, keyZone("a", zoneRow, fmt.Sprintf(" %-9s %s", labelStyle.Render("Attach:"), lipgloss.NewStyle().Foreground(lipgloss.Color("#55ffff")).Render(fmt.Sprintf("%d file(s)", len(m.compose.Attachments))))))

	for i, att := range m.compose.Attachments {
		isSel := (i == m.compose.AttCursor)
		prefix := "    "
		if isSel {
			prefix = "  > "
		}
		attInfo := fmt.Sprintf("[%d] %s (%s)", i+1, att.Filename, formatFileSize(len(att.Data)))
		if isSel {
			style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#333366"))
			lines = append(lines, attZone(i, prefix+style.Render(attInfo)))
		} else {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("#cccccc"))
			lines = append(lines, attZone(i, prefix+style.Render(attInfo)))
		}
	}
	return lines
}

func renderComposeFooter(width ...int) string {
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#55ffff")).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#aaaaaa"))
	sendStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#55ff55")).Bold(true)
	exitStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Bold(true)

	// Each entry is clickable as its key; Send asks first when clicked.
	item := func(zoneKey string, style lipgloss.Style, label, desc string) string {
		return keyZone(zoneKey, zoneFooter, style.Render(label)+" "+descStyle.Render(desc))
	}
	items1 := []string{
		item(composeSendZone, sendStyle, "[y]", "Send"),
		item("e", keyStyle, "[e]", "Edit Body"),
		item("t", keyStyle, "[t]", "To"),
		item("c", keyStyle, "[c]", "Cc"),
		item("b", keyStyle, "[b]", "Bcc"),
		item("s", keyStyle, "[s]", "Subj"),
		item("f4", keyStyle, "[F4]", "AI Subj"),
		item("f5", keyStyle, "[F5]", "Contacts"),
	}
	items2 := []string{
		item("a", keyStyle, "[a]", "Add Att"),
		item("d", keyStyle, "[d]", "Del Att"),
		item("v", keyStyle, "[v]", "View Att"),
		item("p", keyStyle, "[p/P]", "PGP"),
		item("f3", keyStyle, "[F3]", "Account"),
		item("q", exitStyle, "[q/Esc]", "Save & Exit"),
		item("x", exitStyle, "[x]", "Discard"),
	}

	w := 0
	if len(width) > 0 {
		w = width[0]
	}
	if w > 0 && w < 100 {
		return fitHints(w, "  ", append(items1, items2...))
	}
	row1 := " " + strings.Join(items1, "  ")
	row2 := " " + strings.Join(items2, "  ")
	return row1 + "\n" + row2
}

// dialogActionRow renders a dialog's confirm/cancel row: a yes button, spacing,
// and a no button, each clickable as its key.
func dialogActionRow(yLabel, nLabel string, yStyle, nStyle lipgloss.Style) string {
	return keyZone("y", zoneDialog, yStyle.Render("  "+yLabel)) + "    " +
		keyZone("n", zoneDialog, nStyle.Render("  "+nLabel))
}

func renderDiscardConfirmDialog(m *tuiModel) string {
	dialogWidth := min(50, max(20, m.width-4))
	if m.width > 0 && dialogWidth > m.width {
		dialogWidth = m.width
	}
	dialogHeight := min(8, max(5, m.height-2))
	if m.height > 0 && dialogHeight > m.height {
		dialogHeight = m.height
	}

	var content strings.Builder
	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff5555")).Render("         DISCARD DRAFT?         ") + "\n\n")
	content.WriteString("  Are you sure you want to discard this draft?\n")
	content.WriteString("  The local draft file will be deleted.\n\n")
	content.WriteString(dialogActionRow("[y] Yes, Discard", "[n/Esc] Cancel",
		lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Bold(true),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#55ff55"))))

	dialogBox := lipgloss.NewStyle().
		Background(lipgloss.Color("#16162a")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#ff5555")).
		Padding(1, 2).
		Width(dialogWidth).
		Height(dialogHeight).
		Render(content.String())

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(dialogBox)
}

func renderWarnEmptySendDialog(m *tuiModel) string {
	dialogWidth := min(56, max(20, m.width-4))
	if m.width > 0 && dialogWidth > m.width {
		dialogWidth = m.width
	}
	dialogHeight := min(9, max(5, m.height-2))
	if m.height > 0 && dialogHeight > m.height {
		dialogHeight = m.height
	}

	reason := m.compose.WarnEmptyReason
	if reason == "" {
		reason = "Message has empty fields!"
	}

	var content strings.Builder
	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffd700")).Render("      ⚠ WARNING: EMPTY EMAIL FIELDS      ") + "\n\n")
	content.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffaa00")).Render(reason)))
	content.WriteString("  It is strongly advised to provide a subject and body.\n\n")
	content.WriteString(dialogActionRow("[y] Send Anyway", "[n/Esc] Return to Edit",
		lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#55ff55")).Bold(true)))

	dialogBox := lipgloss.NewStyle().
		Background(lipgloss.Color("#16162a")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#ffd700")).
		Padding(1, 2).
		Width(dialogWidth).
		Height(dialogHeight).
		Render(content.String())

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(dialogBox)
}

// sendConfirmSubjectLine shows the subject the message goes out with, and
// says so when the model wrote it: that line is the one the user has not
// read yet.
func sendConfirmSubjectLine(c *composeState) string {
	subject := strings.TrimSpace(c.Subject)
	if subject == "" {
		return "Subject: (none)"
	}
	if c.SubjectGenerated {
		return "Subject (LLM): " + truncateTo(email.BidiDisplay(subject), 36)
	}
	return "Subject: " + truncateTo(email.BidiDisplay(subject), 42)
}

// renderSendConfirmDialog asks before a clicked Send goes out, naming where it
// goes so the answer is an informed one.
func renderSendConfirmDialog(m *tuiModel) string {
	to := m.compose.To
	if to == "" {
		to = "(no recipient)"
	}
	var content strings.Builder
	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55ff55")).Render("          SEND THIS EMAIL?          ") + "\n\n")
	content.WriteString(fmt.Sprintf("  To: %s\n", truncateTo(to, 44)))
	content.WriteString(fmt.Sprintf("  %s\n", sendConfirmSubjectLine(m.compose)))
	content.WriteString(fmt.Sprintf("  %d attachment(s)\n\n", len(m.compose.Attachments)))
	content.WriteString(dialogActionRow("[y] Send", "[n/Esc] Cancel",
		lipgloss.NewStyle().Foreground(lipgloss.Color("#55ff55")).Bold(true),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))))

	dialogWidth := min(54, max(20, m.width-4))
	if m.width > 0 && dialogWidth > m.width {
		dialogWidth = m.width
	}
	dialogHeight := min(9, max(5, m.height-2))
	if m.height > 0 && dialogHeight > m.height {
		dialogHeight = m.height
	}

	dialogBox := lipgloss.NewStyle().
		Background(lipgloss.Color("#16162a")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#55ff55")).
		Padding(1, 2).
		Width(dialogWidth).
		Height(dialogHeight).
		Render(content.String())

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(dialogBox)
}

// handleSendTick re-arms the tick while the session's send is in flight; a
// tick that outlives the send, or belongs to another draft, ends here.
func (m *tuiModel) handleSendTick(msg sendTickMsg) (tea.Model, tea.Cmd) {
	if m.compose != nil && m.compose.sending && m.compose.MessageID == msg.session {
		return m, sendTickCmd(msg.session)
	}
	return m, nil
}

// sendTickInterval is how often the sending dialog refreshes; a variable so
// tests that run the command do not wait a second for it.
var sendTickInterval = time.Second

func sendTickCmd(session string) tea.Cmd {
	return tea.Tick(sendTickInterval, func(time.Time) tea.Msg { return sendTickMsg{session: session} })
}

// renderSendingDialog shows a send in flight with the seconds it has taken.
// A send cannot be recalled once the server has it, so Esc does not cancel
// one; pressing it says so instead of seeming to do nothing.
func renderSendingDialog(width, height int, c *composeState) string {
	elapsed := max(0, int(time.Since(c.sendStart).Seconds()))
	lines := []string{fmt.Sprintf(" Sending message... %ds ", elapsed)}
	if c.sendEscNoted {
		lines = append(lines, "", "A send cannot be canceled; please wait.")
	}
	if c.quitAfterSend {
		lines = append(lines, "", "Exiting once it is sent.")
	}
	return renderStatusDialog(width, height, strings.Join(lines, "\n"))
}
