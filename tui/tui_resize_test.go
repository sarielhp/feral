package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sarielhp/feral/calendar"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/filebrowser"
)

func TestWindowSizeMsg_DetailModeRewraps(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.mode = modeDetail
	m.width, m.height = 100, 30
	m.detail = "This is a long line of email text that must wrap cleanly to the viewport width on each resize."
	m.detailH = "Subject: Test\nFrom: user@example.com"
	m.detailVpDirty = true

	// First render at 100 width
	_ = m.View()
	if m.detailVpDirty {
		t.Errorf("expected detailVpDirty=false after render")
	}

	// Resize to 45 columns
	res, _ := m.Update(tea.WindowSizeMsg{Width: 45, Height: 20})
	tm := res.(*tuiModel)

	if tm.width != 45 || tm.height != 20 {
		t.Fatalf("expected width 45, height 20; got %d, %d", tm.width, tm.height)
	}
	if !tm.detailVpDirty {
		t.Fatalf("expected detailVpDirty=true after WindowSizeMsg")
	}

	// View again - must refresh viewport and reset dirty flag
	view := tm.View()
	if tm.detailVpDirty {
		t.Errorf("expected detailVpDirty=false after View()")
	}
	wantW := max(20, 45-2)
	wantH := max(5, 20-3)
	if tm.vp.Width != wantW || tm.vp.Height != wantH {
		t.Errorf("expected vp (%d, %d), got (%d, %d)", wantW, wantH, tm.vp.Width, tm.vp.Height)
	}
	if view == "" {
		t.Errorf("expected non-empty detail view")
	}
}

func TestWindowSizeMsg_SplitView(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.splitPreview = true
	m.emails = []email.Email{
		{ID: "1", Subject: "First Message", FromEmail: "a@b.com", EmailDate: time.Now()},
		{ID: "2", Subject: "Second Message", FromEmail: "b@b.com", EmailDate: time.Now()},
	}
	m.eIdx = 0

	res, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	tm := res.(*tuiModel)

	if tm.width != 80 || tm.height != 24 {
		t.Fatalf("expected 80x24, got %dx%d", tm.width, tm.height)
	}
	expectedLines := max(3, (24-2-1)/2)
	if tm.indexLines() != expectedLines {
		t.Errorf("expected indexLines=%d, got %d", expectedLines, tm.indexLines())
	}

	view := tm.View()
	if !strings.Contains(view, "Preview") {
		t.Errorf("expected split view preview divider in view:\n%s", view)
	}
}

func TestWindowSizeMsg_Compose(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.mode = modeCompose
	m.compose = &composeState{
		To:      "recipient@example.com",
		Subject: "Test Draft",
		Body:    "Draft line 1\nDraft line 2",
		Vp:      viewport.New(30, 10),
	}

	res, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 35})
	tm := res.(*tuiModel)

	if tm.compose.Vp.Width != 90-2 {
		t.Errorf("expected compose.Vp.Width=%d, got %d", 88, tm.compose.Vp.Width)
	}

	view := tm.View()
	if !strings.Contains(view, "COMPOSE & SEND REVIEW") {
		t.Errorf("expected compose header in view")
	}

	// Resizing to small dimensions
	res2, _ := tm.Update(tea.WindowSizeMsg{Width: 35, Height: 15})
	tm2 := res2.(*tuiModel)
	smallView := tm2.View()
	if smallView == "" {
		t.Errorf("expected non-empty compose view on small size")
	}
}

func TestWindowSizeMsg_FileBrowser(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.fileBrowserOpen = true
	m.fileBrowser = filebrowser.New(t.TempDir(), "target.txt", 80, 24)

	res, _ := m.Update(tea.WindowSizeMsg{Width: 105, Height: 32})
	tm := res.(*tuiModel)

	if tm.fileBrowser.Width != 105 || tm.fileBrowser.Height != 32 {
		t.Errorf("expected fileBrowser size 105x32, got %dx%d", tm.fileBrowser.Width, tm.fileBrowser.Height)
	}

	view := tm.View()
	if view == "" {
		t.Errorf("expected non-empty view with file browser")
	}
}

func TestWindowSizeMsg_Modals(t *testing.T) {
	// 1. Attachment preview modal
	m := newTuiModel(nil, "", nil)
	m.attModalOpen = true
	m.attPreviewAtt = email.Attachment{
		Filename:    "test.txt",
		ContentType: "text/plain",
		Data:        []byte("Hello world attachment"),
	}
	m.attachments = []email.Attachment{m.attPreviewAtt}
	m.attCursor = 0

	res, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 25})
	tm := res.(*tuiModel)
	if tm.attPreviewVp.Width <= 0 || tm.attPreviewVp.Height <= 0 {
		t.Errorf("expected positive attPreviewVp dimensions, got (%d, %d)", tm.attPreviewVp.Width, tm.attPreviewVp.Height)
	}
	view := tm.View()
	if !strings.Contains(view, "ATTACHMENT PREVIEW") {
		t.Errorf("expected attachment modal view")
	}

	// 2. ICS modal
	m2 := newTuiModel(nil, "", nil)
	m2.icsModalOpen = true
	m2.icsEvent = &calendar.ICSEvent{
		Summary:     "Team Sync",
		Description: "A long meeting description that wraps inside the viewport",
	}
	m2.icsEvents = []*calendar.ICSEvent{m2.icsEvent}
	res2, _ := m2.Update(tea.WindowSizeMsg{Width: 65, Height: 22})
	tm2 := res2.(*tuiModel)
	if tm2.icsVp.Width <= 0 {
		t.Errorf("expected positive icsVp width, got %d", tm2.icsVp.Width)
	}
	icsView := tm2.View()
	if !strings.Contains(icsView, "CALENDAR EVENT") {
		t.Errorf("expected ics modal view")
	}

	// 3. Summary modal
	m3 := newTuiModel(nil, "", nil)
	m3.showSummary = true
	m3.summaryContent = "This email discusses project timeline and deliverables."
	res3, _ := m3.Update(tea.WindowSizeMsg{Width: 75, Height: 28})
	tm3 := res3.(*tuiModel)
	if tm3.summaryVp.Width <= 0 || tm3.summaryVp.Height <= 0 {
		t.Errorf("expected positive summaryVp dimensions, got (%d, %d)", tm3.summaryVp.Width, tm3.summaryVp.Height)
	}
	sumView := tm3.View()
	if !strings.Contains(sumView, "EMAIL SUMMARY") {
		t.Errorf("expected summary overlay view")
	}

	// 4. Attachment viewer
	m4 := newTuiModel(nil, "", nil)
	m4.attachmentListOpen = true
	m4.attachmentViewerOpen = true
	m4.attachments = []email.Attachment{
		{Filename: "code.go", Data: []byte("package main\n\nfunc main() {}\n")},
	}
	m4.attachmentListCursor = 0
	res4, _ := m4.Update(tea.WindowSizeMsg{Width: 85, Height: 30})
	tm4 := res4.(*tuiModel)
	if tm4.attachmentViewerVp.Width <= 0 {
		t.Errorf("expected positive attachmentViewerVp width, got %d", tm4.attachmentViewerVp.Width)
	}
	codeView := tm4.View()
	if !strings.Contains(codeView, "Attachment: code.go") {
		t.Errorf("expected attachment viewer view")
	}
}

func TestWindowSizeMsg_SmallTerminalStress(t *testing.T) {
	sizes := []struct {
		w, h int
	}{
		{25, 10},
		{20, 8},
		{15, 6},
	}

	for _, sz := range sizes {
		checkView := func(name string, v string) {
			if v == "" {
				t.Errorf("%s returned empty view for size %dx%d", name, sz.w, sz.h)
			}
		}

		// Test Compose Confirm Modals
		m := newTuiModel(nil, "", nil)
		m.mode = modeCompose
		m.width, m.height = sz.w, sz.h
		m.compose = &composeState{
			To:             "to@example.com",
			Subject:        "Subj",
			DiscardConfirm: true,
		}
		checkView("DiscardConfirm", m.View())

		m.compose.DiscardConfirm = false
		m.compose.SendConfirm = true
		checkView("SendConfirm", m.View())

		m.compose.SendConfirm = false
		m.compose.WarnEmptySend = true
		checkView("WarnEmptySend", m.View())

		// Test Sort Modal
		mSort := newTuiModel(nil, "", nil)
		mSort.width, mSort.height = sz.w, sz.h
		mSort.sortMenuOpen = true
		checkView("SortModal", mSort.View())

		// Test Spam Menu Modal
		mSpam := newTuiModel(nil, "", nil)
		mSpam.width, mSpam.height = sz.w, sz.h
		mSpam.spamMenuOpen = true
		checkView("SpamMenu", mSpam.View())

		// Test Block Modal
		mBlock := newTuiModel(nil, "", nil)
		mBlock.width, mBlock.height = sz.w, sz.h
		mBlock.blockModalOpen = true
		mBlock.blockSenderEmail = "spammer@evil.com"
		checkView("BlockModal", mBlock.View())

		// Test Deadlines Modal
		mDeadlines := newTuiModel(nil, "", nil)
		mDeadlines.width, mDeadlines.height = sz.w, sz.h
		mDeadlines.showDeadlinesAgenda = true
		mDeadlines.deadlinesList = []deadlineEntry{
			{Description: "Pay taxes", Subject: "Tax notice", Sender: "irs@gov"},
		}
		checkView("DeadlinesModal", mDeadlines.View())

		// Test Virtual Browser
		mVirt := newTuiModel(nil, "", nil)
		mVirt.width, mVirt.height = sz.w, sz.h
		mVirt.virtualBrowserOpen = true
		checkView("VirtualBrowser", mVirt.View())

		// Test Account Overlay
		mAcc := newTuiModel(nil, "", nil)
		mAcc.width, mAcc.height = sz.w, sz.h
		mAcc.accountOverlayOpen = true
		checkView("AccountOverlay", mAcc.View())
	}
}
