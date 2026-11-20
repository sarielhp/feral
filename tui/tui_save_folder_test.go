package tui

import (
	"errors"
	"github.com/sarielhp/feral/cache/cachedir"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

func setupTestSaveFolderModel(t *testing.T) (*tuiModel, *mailclient.MockMailClient) {
	t.Helper()
	tempDir := t.TempDir()
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir)}
	mock := &mailclient.MockMailClient{Cfg: cfg, InboxFolderVal: "INBOX"}
	m := newTuiModel(mock, "INBOX", nil)
	m.width = 80
	m.height = 24
	m.globalFolders = []label.Item{
		{Name: "INBOX", FullName: "INBOX"},
		{Name: "Archive", FullName: "Archive"},
		{Name: "Work", FullName: "Work"},
	}
	m.folders = m.globalFolders
	m.globalTree = label.BuildLabelTree(m.globalFolders)
	m.tree = m.globalTree
	m.rawEmails = []email.Email{
		{ID: "msg-1", Subject: "Important Project", EmailDate: time.Now()},
		{ID: "msg-2", Subject: "Weekly Update", EmailDate: time.Now()},
	}
	m.rebuildVisibleEmails()
	m.eIdx = 0
	return m, mock
}

func TestTUI_SaveMessageToFolder_IndexMode(t *testing.T) {
	m, mock := setupTestSaveFolderModel(t)

	// 1. Press 's' in Index mode
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	tm := model.(*tuiModel)

	if !tm.treeOpen {
		t.Fatalf("expected treeOpen=true after 's'")
	}
	if !tm.savingMessageToFolder {
		t.Fatalf("expected savingMessageToFolder=true after 's'")
	}
	if tm.savingMessageID != "msg-1" {
		t.Fatalf("expected savingMessageID='msg-1', got %q", tm.savingMessageID)
	}

	// 2. Cancel with 'Esc'
	model, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tm = model.(*tuiModel)
	if tm.treeOpen {
		t.Fatalf("expected treeOpen=false after Esc")
	}
	if tm.savingMessageToFolder {
		t.Fatalf("expected savingMessageToFolder=false after Esc")
	}

	// 3. Press 's' again, navigate to 'Archive', and press Enter
	model, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	tm = model.(*tuiModel)

	// Find Archive in entries
	found := false
	for i, e := range tm.entries {
		if e.Node != nil && e.Node.FullName == "Archive" {
			tm.cursor = i
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("folder 'Archive' not found in entries: %+v", tm.entries)
	}

	model, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = model.(*tuiModel)

	if tm.treeOpen {
		t.Fatalf("expected treeOpen=false after Enter")
	}
	if tm.savingMessageToFolder {
		t.Fatalf("expected savingMessageToFolder=false after Enter")
	}
	if cmd == nil {
		t.Fatalf("expected non-nil cmd from saveEmailToFolder")
	}

	// Execute cmd
	msg := cmd()
	doneMsg, ok := msg.(saveEmailDoneMsg)
	if !ok {
		t.Fatalf("expected saveEmailDoneMsg, got %T", msg)
	}
	if doneMsg.err != nil {
		t.Fatalf("unexpected error in saveEmailDoneMsg: %v", doneMsg.err)
	}
	if doneMsg.id != "msg-1" || doneMsg.folder != "Archive" {
		t.Fatalf("expected msg-1 to Archive, got %s to %s", doneMsg.id, doneMsg.folder)
	}

	// Verify MockMailClient recorded the copy
	if len(mock.CopiedEmails) != 1 {
		t.Fatalf("expected mock to record 1 copy, got: %+v", mock.CopiedEmails)
	}
	record := mock.CopiedEmails[0]
	if record.DestLabel != "Archive" || len(record.MessageIDs) != 1 || record.MessageIDs[0] != "msg-1" {
		t.Fatalf("expected copy of msg-1 to Archive, got: %+v", record)
	}

	// Verify update message handles status
	model, _ = tm.Update(doneMsg)
	tm = model.(*tuiModel)
	if !strings.Contains(tm.saveStatus, "Saved message to Archive") {
		t.Fatalf("expected saveStatus 'Saved message to Archive', got %q", tm.saveStatus)
	}
}

func TestTUI_SaveMessageToFolder_AlreadyInFolder(t *testing.T) {
	m, _ := setupTestSaveFolderModel(t)
	dir := m.Config().AccountCache

	// Pre-seed folder index with msg-1 already in Archive
	if err := folderindex.ReplaceAll(dir, "Archive", []email.ID{"msg-1"}); err != nil {
		t.Fatalf("seeding Archive index: %v", err)
	}

	// Press 's'
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	tm := model.(*tuiModel)

	// Select Archive
	for i, e := range tm.entries {
		if e.Node != nil && e.Node.FullName == "Archive" {
			tm.cursor = i
			break
		}
	}

	model, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = model.(*tuiModel)

	if cmd != nil {
		t.Fatalf("expected nil cmd when message already in folder, got %v", cmd)
	}
	if !strings.Contains(tm.saveStatus, "Message already in Archive") {
		t.Fatalf("expected saveStatus 'Message already in Archive', got %q", tm.saveStatus)
	}
}

func TestTUI_SaveMessageToFolder_DetailMode(t *testing.T) {
	m, mock := setupTestSaveFolderModel(t)
	m.mode = modeDetail
	m.selectedID = "msg-2"

	// Press 's' in Detail mode
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	tm := model.(*tuiModel)

	if !tm.treeOpen || !tm.savingMessageToFolder {
		t.Fatalf("expected treeOpen and savingMessageToFolder true")
	}
	if tm.savingMessageID != "msg-2" {
		t.Fatalf("expected savingMessageID='msg-2', got %q", tm.savingMessageID)
	}

	// Select Work folder
	for i, e := range tm.entries {
		if e.Node != nil && e.Node.FullName == "Work" {
			tm.cursor = i
			break
		}
	}

	model, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = model
	if cmd == nil {
		t.Fatalf("expected non-nil cmd")
	}

	msg := cmd()
	doneMsg := msg.(saveEmailDoneMsg)
	if doneMsg.err != nil {
		t.Fatalf("unexpected error: %v", doneMsg.err)
	}

	if len(mock.CopiedEmails) != 1 {
		t.Fatalf("expected 1 copy recorded, got %+v", mock.CopiedEmails)
	}
	record := mock.CopiedEmails[0]
	if record.DestLabel != "Work" || len(record.MessageIDs) != 1 || record.MessageIDs[0] != "msg-2" {
		t.Fatalf("expected msg-2 copied to Work, got: %+v", record)
	}
}

func TestTUI_SaveF2AndF3FlipAndBlockedKeys(t *testing.T) {
	m, mock := setupTestSaveFolderModel(t)
	mock.RawMessages = map[email.ID][]byte{"msg-1": []byte("Subject: test\r\n\r\nhello")}

	// 1. Press F2 in Index mode -> opens Save to Disk (returns fetch command)
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m = model.(*tuiModel)
	if cmd != nil {
		msg := cmd()
		model, _ = m.Update(msg)
		m = model.(*tuiModel)
	}
	if !m.fileBrowserOpen || !m.savingMessageToDisk {
		t.Fatalf("expected fileBrowserOpen and savingMessageToDisk true after F2")
	}

	// 2. Press blocked keys in save to disk -> should have no effect
	press(m, "x")
	if !m.fileBrowserOpen || !m.savingMessageToDisk {
		t.Fatalf("expected blocked keys to not exit save to disk")
	}

	// 3. Press F1 -> shows active keys help
	press(m, "f1")
	if !m.saveHelpOpen {
		t.Fatal("expected saveHelpOpen to be true after pressing F1")
	}
	press(m, "esc")
	if m.saveHelpOpen {
		t.Fatal("expected saveHelpOpen to close after pressing Esc")
	}

	// 4. Press F3 -> flips to Save to Label!
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyF3})
	m = model.(*tuiModel)
	if m.fileBrowserOpen || m.savingMessageToDisk {
		t.Fatal("expected fileBrowserOpen and savingMessageToDisk false after F3")
	}
	if !m.treeOpen || !m.savingMessageToFolder {
		t.Fatal("expected treeOpen and savingMessageToFolder true after F3")
	}

	// 5. In Save to Label, press F1 -> shows active keys help
	press(m, "f1")
	if !m.saveHelpOpen {
		t.Fatal("expected saveHelpOpen true in save to label after F1")
	}
	press(m, "f1")
	if m.saveHelpOpen {
		t.Fatal("expected saveHelpOpen false after pressing F1 again")
	}

	// 6. In Save to Label, press blocked keys -> no effect
	press(m, "x")
	press(m, "5")
	if !m.treeOpen || !m.savingMessageToFolder {
		t.Fatal("expected blocked keys to not exit save to label")
	}

	// 7. Press F3 -> flips back to Save to Disk!
	model, cmd = m.Update(tea.KeyMsg{Type: tea.KeyF3})
	m = model.(*tuiModel)
	if cmd != nil {
		msg := cmd()
		model, _ = m.Update(msg)
		m = model.(*tuiModel)
	}
	if !m.fileBrowserOpen || !m.savingMessageToDisk {
		t.Fatal("expected fileBrowserOpen and savingMessageToDisk true after flip back to disk")
	}

	// 8. Press F3 -> flips to Save to Label!
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyF3})
	m = model.(*tuiModel)

	// 9. Press F2 -> saves to selected folder!
	model, cmd = m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m = model.(*tuiModel)
	if m.treeOpen || m.savingMessageToFolder {
		t.Fatal("expected treeOpen and savingMessageToFolder false after F2 save")
	}
	if cmd == nil {
		t.Fatal("expected non-nil save cmd after F2")
	}
}

func TestTUI_SaveFolderModalViewLook(t *testing.T) {
	m, mock := setupTestSaveFolderModel(t)
	mock.RawMessages = map[email.ID][]byte{"msg-1": []byte("Subject: Important Project\r\n\r\nBody")}
	m.selectedID = "msg-1"

	// Press 's' to enter Save to Label view
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = model.(*tuiModel)

	if !m.savingMessageToFolder {
		t.Fatal("expected savingMessageToFolder=true")
	}

	view := m.View()
	if !strings.Contains(view, "SELECT SAVE LABEL / FOLDER") {
		t.Errorf("expected view to contain header 'SELECT SAVE LABEL / FOLDER', got:\n%s", view)
	}
	if !strings.Contains(view, "Target Folder:") {
		t.Errorf("expected view to contain 'Target Folder:', got:\n%s", view)
	}
	if !strings.Contains(view, "[Enter/F2] Save to Label") {
		t.Errorf("expected view to contain '[Enter/F2] Save to Label', got:\n%s", view)
	}
	if !strings.Contains(view, "[F3] Save to Disk") {
		t.Errorf("expected view to contain '[F3] Save to Disk', got:\n%s", view)
	}

	// Flip to Save to Disk with F3
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF3})
	m = model.(*tuiModel)
	if cmd != nil {
		msg := cmd()
		model, _ = m.Update(msg)
		m = model.(*tuiModel)
	}

	diskView := m.View()
	if !strings.Contains(diskView, "SELECT SAVE DIRECTORY") {
		t.Errorf("expected disk view to contain 'SELECT SAVE DIRECTORY', got:\n%s", diskView)
	}

	// Flip back to Save to Label with F3
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyF3})
	m = model.(*tuiModel)
	labelView := m.View()
	if !strings.Contains(labelView, "SELECT SAVE LABEL / FOLDER") {
		t.Errorf("expected label view after flip back to contain 'SELECT SAVE LABEL / FOLDER', got:\n%s", labelView)
	}
}

func TestHandleMessageReadyToSave_FetchFailureClearsSaveState(t *testing.T) {
	m, _ := setupTestSaveFolderModel(t)
	m.savingMessageToDisk = true
	m.savingMessageID = "msg-1"

	m.handleMessageReadyToSave(messageReadyToSaveMsg{err: errors.New("offline")})
	if m.savingMessageToDisk || m.savingMessageID != "" {
		t.Errorf("save state left set after a failed fetch: disk=%v id=%q", m.savingMessageToDisk, m.savingMessageID)
	}
	if !m.showError {
		t.Error("the failure was not reported")
	}
}
