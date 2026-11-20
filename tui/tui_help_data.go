package tui

// The help tree's data: the keys of each screen, in the section that
// describes it. A key that works on several screens is listed on each. The
// overlay is never a flat wall of keys: the top level is these sections, and
// each screen opens the section that describes it. TestHelp_RowsNameBoundKeys
// fails when a row names a key that no handler answers to.

// helpRow is one key binding: the key (or keys) and what they do.
type helpRow struct {
	keys string
	desc string
}

// helpSection is one page of the help tree. Which screen opens which page is
// decided by contextualHelpSection.
type helpSection struct {
	name string
	desc string // the one line the section menu shows
	rows []helpRow
}

// helpSections are the pages, in menu order.
var helpSections = []helpSection{
	{
		name: "Messages",
		desc: "the list: move, act, tag",
		rows: []helpRow{
			{"j / k, ↑ / ↓", "Move down / up"},
			{"Home / End", "First / last email"},
			{"PgUp / PgDn", "Page up / down"},
			{"Space", "Toggle split preview"},
			{"← / →", "Prev / next email (split)"},
			{"↑ / ↓", "Scroll preview (split)"},
			{"Tab", "Unfold thread (10 rows) / fold"},
			{"e / Enter", "Read full email"},
			{"w", "Open in web browser"},
			{"c", "Compose new email"},
			{"r / g / G", "Reply / group / to the list"},
			{"b", "Forward email"},
			{"E", "Archive email"},
			{"H", "Hide (snooze) until a time"},
			{"d / z", "Delete / undo last move"},
			{"N", "Toggle read / unread"},
			{"x", "Mark all in folder read"},
			{"S", "Mark as spam"},
			{"U / u", "Unspam / undo spam"},
			{"B", "Block sender (Spam/Trash)"},
			{"M / X", "Mute thread / leave list"},
			{"s", "Save message to folder"},
			{"F2", "Save message (disk / label)"},
			{"o / O", "Sort / reverse-sort folder"},
			{"t / T / ;", "Tag / pattern / prefix"},
			{"; v / x", "Save tagged set / to mbox"},
			{"; a", "Move tagged to acct:folder"},
			{"Ctrl+a / u", "Tag all / clear tags"},
			{"v", "Attachments & parts"},
			{"a", "Alias the sender"},
			{"F5", "Spam menu"},
			{"F6", "Aliases — view & add"},
			{"F7", "Saved searches"},
			{"F8", "Recent folders"},
			{"Shift+← / →", "Back / forward a view"},
			{"R", "Refresh folder"},
			{"L", "Lock folder (read-only)"},
			{"f / F", "Toggle folder tree"},
			{"|", "Pipe message to a command"},
		},
	},
	{
		name: "Reading",
		desc: "the open message",
		rows: []helpRow{
			{"j / k, ↑ / ↓", "Scroll the message"},
			{"PgUp / PgDn", "Page the message"},
			{"Home / End", "Top / bottom"},
			{"← / →", "Prev / next email"},
			{"/", "Search the body"},
			{"w", "Open in web browser"},
			{"E / d / S", "Archive / delete / spam"},
			{"H", "Hide (snooze) until a time"},
			{"z", "Undo last move (last ten)"},
			{"N", "Toggle read / unread"},
			{"x", "Mark all in folder read"},
			{"B", "Block sender"},
			{"X", "Unsubscribe from the list"},
			{"U / u", "Unspam / undo spam"},
			{"s", "Save message to folder"},
			{"v", "Attachments & parts"},
			{"a", "Alias the sender"},
			{"Tab / p", "Focus / hide the right pane"},
			{"F2", "Save message (disk / label)"},
			{"F4", "Summarize email (LLM)"},
			{"r / g / G", "Reply / group / to the list"},
			{"b", "Forward email"},
			{"M", "Mute / unmute the thread"},
			{"F6", "Aliases — view & add"},
			{"F7", "Saved searches"},
			{"F8", "Recent folders"},
			{"Shift+← / →", "Previous / next view"},
			{"Esc", "Back to the list"},
		},
	},
	{
		name: "Compose",
		desc: "writing and sending",
		rows: []helpRow{
			{"y", "Send email"},
			{"t / c / b", "Edit To / Cc / Bcc"},
			{"s", "Edit subject"},
			{"F4", "Generate subject with LLM"},
			{"e", "Edit body ($EDITOR)"},
			{"Tab", "Complete recipient / next att."},
			{"a / d / v", "Add / delete / view attachment"},
			{"p", "Sign this message (PGP)"},
			{"P", "Encrypt this message (PGP)"},
			{"F5", "Pick recipients from contacts"},
			{"F6", "Aliases — view, search, add"},
			{"F3", "Choose sending account"},
			{"q / Esc", "Save draft & exit"},
			{"Ctrl+C", "Save draft & exit"},
			{"x", "Discard draft"},
		},
	},
	{
		name: "Search",
		desc: "filters, fuzzy and remote search",
		rows: []helpRow{
			{"/", "Search/filter emails"},
			{"=", "Only this list (or /list:<id>)"},
			{"~", "Fuzzy search (all accounts)"},
			{"!", "Fuzzy search (this folder)"},
			{"Ctrl+f", "Search the server (newest 25)"},
			{"+", "In search results: load more"},
			{"Tab", "Return to the active filter"},
			{"Esc", "Clear filter / close"},
		},
	},
	{
		name: "Folders",
		desc: "the tree, accounts, archiving",
		rows: []helpRow{
			{"f", "Toggle the folder tree"},
			{"j / k, ↑ / ↓", "Move in the tree"},
			{"Enter", "Open the selected folder"},
			{"/", "Search folders"},
			{"F3", "Switch account"},
			{"F8", "Recent folders"},
			{"L", "Lock folder (read-only)"},
			{"R", "Refresh folder"},
			{"F2", "Save message as .eml"},
		},
	},
	{
		name: "Contacts",
		desc: "aliases and the address picker",
		rows: []helpRow{
			{"F5", "Pick recipients (compose)"},
			{"type", "Search contacts & history"},
			{"↑ / ↓", "Move (PgUp / PgDn page)"},
			{"Space", "Mark one — Enter adds all"},
			{"Tab", "Target To → Cc → Bcc"},
			{"F6", "Aliases browser"},
			{"type", "Search saved aliases"},
			{"Ctrl+n", "New alias: address, then name"},
			{"Enter", "Open entry (insert in picker)"},
			{"c / l", "Compose / latest mail (entry)"},
			{"e", "Edit or add the alias (entry)"},
			{"Esc", "Back — picker or close"},
		},
	},
	{
		name: "Tools",
		desc: "overlays, panels and the mouse",
		rows: []helpRow{
			{"m / Alt", "Menu (spam, account, exit)"},
			{"F4", "Summarize email (LLM)"},
			{"D", "Upcoming deadlines"},
			{"Ctrl+d", "Diagnostics panel"},
			{"F9", "Mouse on/off"},
		},
	},
	{
		name: "General",
		desc: "help, quitting, read-only",
		rows: []helpRow{
			{"F1 / h / ?", "Open this help"},
			{"/", "Search help (Esc clears)"},
			{"Esc", "Back a page, or close it"},
			{"q", "Close help; outside, quit"},
			{"Esc", "On the list: back a view, then quit?"},
			{"Ctrl+q", "Quit from anywhere"},
			{"Ctrl+C", "Quit"},
			{"[RO]", "Read-only: server actions simulated"},
		},
	},
}

// contextualHelpSection is the page the current screen opens help on,
// checking from the most specific overlay to the plain screens.
func contextualHelpSection(m *tuiModel) int {
	switch {
	case m.contactPick != nil || m.aliasView != nil:
		return sectionIndex("Contacts")
	case m.mode == modeCompose:
		return sectionIndex("Compose")
	case m.treeOpen:
		return sectionIndex("Folders")
	case m.inSearch && m.mode != modeDetail:
		return sectionIndex("Search")
	case m.mode == modeDetail:
		return sectionIndex("Reading")
	default:
		return sectionIndex("Messages")
	}
}

// sectionIndex is where a named section sits in the menu, or -1.
func sectionIndex(name string) int {
	for i, sec := range helpSections {
		if sec.name == name {
			return i
		}
	}
	return -1
}
