package tui

import (
	"sort"
	"strings"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
)

func transferExpandedStates(oldNodes, newNodes []*label.TreeNode) {
	oldStates := make(map[string]bool)
	var collect func(nodes []*label.TreeNode)
	collect = func(nodes []*label.TreeNode) {
		for _, n := range nodes {
			oldStates[n.FullName] = n.Expanded
			collect(n.Children)
		}
	}
	collect(oldNodes)
	var apply func(nodes []*label.TreeNode)
	apply = func(nodes []*label.TreeNode) {
		for _, n := range nodes {
			if exp, ok := oldStates[n.FullName]; ok {
				n.Expanded = exp
			}
			apply(n.Children)
		}
	}
	apply(newNodes)
}

func (m *tuiModel) buildEntries() {
	var tree []*label.TreeNode
	if m.inFolderSearch && m.folderSearch != "" {
		var filtered []label.Item
		lowerSearch := strings.ToLower(m.folderSearch)
		sourceFolders := m.folders
		if m.showGlobalTree {
			sourceFolders = m.globalFolders
		}
		for _, f := range sourceFolders {
			if strings.Contains(strings.ToLower(f.FullName), lowerSearch) {
				filtered = append(filtered, f)
			}
		}
		tree = label.BuildLabelTree(filtered)
		var expandAll func(nodes []*label.TreeNode)
		expandAll = func(nodes []*label.TreeNode) {
			for _, n := range nodes {
				n.Expanded = true
				expandAll(n.Children)
			}
		}
		expandAll(tree)
	} else {
		if m.showGlobalTree {
			tree = m.globalTree
		} else {
			tree = m.tree
		}
	}
	m.entries = label.FlattenTree(tree)
	if m.inFolderSearch && m.folderSearch != "" {
		found := false
		for i, entry := range m.entries {
			if entry.Node != nil && len(entry.Node.Children) == 0 {
				m.cursor = i
				found = true
				break
			}
		}
		if !found {
			m.cursor = min(m.cursor, max(0, len(m.entries)-1))
		}
	} else {
		m.cursor = min(m.cursor, max(0, len(m.entries)-1))
	}
}

func (m *tuiModel) sortThreadRoots(roots []*email.ThreadNode) {
	isBadNode := func(n *email.ThreadNode) bool {
		return n.Email.IsSpam || n.Email.IsPolitical || n.Email.IsBlacklisted
	}
	sort.Slice(roots, func(i, j int) bool {
		badI := isBadNode(roots[i])
		badJ := isBadNode(roots[j])
		if badI != badJ {
			if m.sortReverse {
				return badI
			}
			return !badI
		}
		dateI := email.GetThreadNewestDate(roots[i])
		dateJ := email.GetThreadNewestDate(roots[j])
		if m.sortReverse {
			return dateI.After(dateJ)
		}
		return dateI.Before(dateJ)
	})
}

func buildThreadIDOrder(roots []*email.ThreadNode) map[email.ID]int {
	var expandedOrder []email.ID
	var getExpandedOrder func(n *email.ThreadNode)
	getExpandedOrder = func(n *email.ThreadNode) {
		expandedOrder = append(expandedOrder, n.Email.ID)
		for _, child := range n.Children {
			getExpandedOrder(child)
		}
	}
	for _, root := range roots {
		getExpandedOrder(root)
	}
	idToNumber := make(map[email.ID]int, len(expandedOrder))
	for i, id := range expandedOrder {
		idToNumber[id] = i + 1
	}
	return idToNumber
}

func computeThreadPrefix(depth int, isLastSlice []bool, hasReplies bool, isCollapsed bool) string {
	if depth == 0 {
		if !hasReplies {
			return ""
		}
		if isCollapsed {
			return "━━━ "
		}
		return "┌── "
	}
	var sb strings.Builder
	for j := 0; j < depth-1; j++ {
		if isLastSlice[j] {
			sb.WriteString("  ")
		} else {
			sb.WriteString("│ ")
		}
	}
	if isLastSlice[depth-1] {
		sb.WriteString("└── ")
	} else {
		sb.WriteString("├── ")
	}
	return sb.String()
}

func collectThreadSenders(n *email.ThreadNode) string {
	var senders []string
	seen := make(map[string]bool)
	var collect func(node *email.ThreadNode)
	collect = func(node *email.ThreadNode) {
		name := node.Email.SenderColumn(false)
		if !seen[name] {
			seen[name] = true
			senders = append(senders, name)
		}
		for _, child := range node.Children {
			collect(child)
		}
	}
	collect(n)
	return strings.Join(senders, ", ")
}

func collectThreadCCs(n *email.ThreadNode) string {
	var parts []string
	seen := make(map[string]bool)
	var collect func(node *email.ThreadNode)
	collect = func(node *email.ThreadNode) {
		if node.Email == nil {
			return
		}
		for _, item := range []string{
			node.Email.CcRaw,
			node.Email.CcEmail,
			node.Email.ToRaw,
			node.Email.ToEmail,
			node.Email.FromRaw,
			node.Email.FromEmail,
		} {
			if item != "" && !seen[item] {
				seen[item] = true
				parts = append(parts, item)
			}
		}
		for _, child := range node.Children {
			collect(child)
		}
	}
	collect(n)
	return strings.Join(parts, " ")
}

func emailMatchesQuery(em *email.Email, lowerQuery string) bool {
	if matched, isList := matchesListFilter(em, lowerQuery); isList {
		return matched
	}
	return strings.Contains(strings.ToLower(em.Subject), lowerQuery) ||
		strings.Contains(strings.ToLower(em.FromRaw), lowerQuery) ||
		strings.Contains(strings.ToLower(em.FromEmail), lowerQuery) ||
		strings.Contains(strings.ToLower(em.ToRaw), lowerQuery) ||
		strings.Contains(strings.ToLower(em.ToEmail), lowerQuery) ||
		strings.Contains(strings.ToLower(em.CcRaw), lowerQuery) ||
		strings.Contains(strings.ToLower(em.CcEmail), lowerQuery) ||
		strings.Contains(strings.ToLower(em.ThreadSenderSummary), lowerQuery) ||
		strings.Contains(strings.ToLower(em.ThreadCCSummary), lowerQuery)
}

func filterVisibleEmails(emails []email.Email, query string, fuzzy bool) []email.Email {
	if query == "" {
		return emails
	}
	if fuzzy {
		return fuzzyFilterEmails(query, emails)
	}
	var filtered []email.Email
	lowerQuery := strings.ToLower(query)
	for _, em := range emails {
		if emailMatchesQuery(&em, lowerQuery) {
			filtered = append(filtered, em)
		}
	}
	return filtered
}

func (m *tuiModel) rebuildVisibleEmails() {
	if len(m.rawEmails) == 0 {
		m.emails = nil
		return
	}
	m.markOwnMail()
	m.applySessionFlags()
	if m.sortOption != SortThreads {
		m.rebuildFlatEmails()
		return
	}
	m.rebuildThreadedEmails()
}

func (m *tuiModel) rebuildFlatEmails() {
	flat := make([]email.Email, len(m.rawEmails))
	copy(flat, m.rawEmails)
	sortEmailsFlat(flat, m.sortOption, m.sortReverse)
	for i := range flat {
		flat[i].ThreadDepth = 0
		flat[i].ThreadPrefix = ""
		flat[i].ThreadHasReplies = false
		flat[i].ThreadRepliesCount = 0
		flat[i].ThreadCollapsed = false
		flat[i].ThreadHiddenUnread = false
		flat[i].ThreadIndex = i + 1
	}
	m.emails = filterVisibleEmails(flat, m.searchQuery, m.fuzzySearch)
}

func (m *tuiModel) rebuildThreadedEmails() {
	roots := email.BuildThreadTree(m.rawEmails)
	m.sortThreadRoots(roots)
	idToNumber := buildThreadIDOrder(roots)

	var visible []email.Email
	var flatten func(n *email.ThreadNode, depth int, isAncestorCollapsed bool, isLastSlice []bool)
	flatten = func(n *email.ThreadNode, depth int, isAncestorCollapsed bool, isLastSlice []bool) {
		if isAncestorCollapsed {
			for i, child := range n.Children {
				flatten(child, depth+1, true, append(isLastSlice, i == len(n.Children)-1))
			}
			return
		}
		em := *n.Email
		em.ThreadDepth = depth
		em.ThreadHasReplies = len(n.Children) > 0
		em.ThreadRepliesCount = email.CountThreadReplies(n)
		em.ThreadIndex = idToNumber[em.ID]
		isCollapsed := !m.expandedThreads[threadKeyOf(&em)]
		em.ThreadCollapsed = isCollapsed
		em.ThreadPrefix = computeThreadPrefix(depth, isLastSlice, em.ThreadHasReplies, isCollapsed)
		em.ThreadHiddenUnread = isCollapsed && hasUnreadReply(n)
		if em.ThreadHasReplies {
			em.ThreadSenderSummary = collectThreadSenders(n)
			em.ThreadCCSummary = collectThreadCCs(n)
		}
		visible = append(visible, em)
		for i, child := range n.Children {
			flatten(child, depth+1, isCollapsed, append(isLastSlice, i == len(n.Children)-1))
		}
	}
	for _, root := range roots {
		flatten(root, 0, false, []bool{})
	}
	m.emails = filterVisibleEmails(visible, m.searchQuery, m.fuzzySearch)
}

// applySessionFlags writes the read and replied state the session holds onto
// rawEmails, which every rebuild of the list starts from. The key handlers
// mark the visible row and the maps; without this, the next rebuild -- a
// delete, a sort, a search, expanding a thread -- copied the stale row back
// and an opened message showed N again. Only IDs the maps know are touched:
// in the unified inbox the maps are the active account's, and a row from
// another account keeps what its own loader set.
func (m *tuiModel) applySessionFlags() {
	for i := range m.rawEmails {
		id := m.rawEmails[i].ID
		if v, ok := m.isRead[id]; ok {
			m.rawEmails[i].IsRead = v
		}
		if v, ok := m.isReplied[id]; ok {
			m.rawEmails[i].IsReplied = v
		}
	}
}

// flagEdit is a read or replied mark made in this session, stamped with the
// model's flagGen when it was made.
type flagEdit struct {
	gen      uint64
	setsRead bool
	read     bool
	replied  bool
}

// noteRead records a read mark the user made: in the session's map, and as an
// edit a folder load already in flight must not undo.
func (m *tuiModel) noteRead(id email.ID, read bool) {
	m.isRead[id] = read
	e := m.nextFlagEdit(id)
	e.setsRead, e.read = true, read
	m.flagEdits[id] = e
}

// noteReplied records that the user replied to id. See noteRead.
func (m *tuiModel) noteReplied(id email.ID) {
	m.isReplied[id] = true
	e := m.nextFlagEdit(id)
	e.replied = true
	m.flagEdits[id] = e
}

func (m *tuiModel) nextFlagEdit(id email.ID) flagEdit {
	if m.flagEdits == nil {
		m.flagEdits = make(map[email.ID]flagEdit)
	}
	m.flagGen++
	e := m.flagEdits[id]
	e.gen = m.flagGen
	return e
}

// reapplyFlagEdits lays the marks made since a load was dispatched over the
// maps it brought. The load read the read state before they reached the disk,
// and replacing the maps with its snapshot showed an opened message as N
// again. Marks the load came after are dropped: they are on disk, and the
// pending-reads ledger kept the sync from undoing them.
func (m *tuiModel) reapplyFlagEdits(loadGen uint64) {
	for id, e := range m.flagEdits {
		if e.gen <= loadGen {
			delete(m.flagEdits, id)
			continue
		}
		if e.setsRead {
			m.isRead[id] = e.read
		}
		if e.replied {
			m.isReplied[id] = true
		}
	}
}

// hasUnreadReply reports whether any message below n is unread.
func hasUnreadReply(n *email.ThreadNode) bool {
	for _, child := range n.Children {
		if !child.Email.IsRead || hasUnreadReply(child) {
			return true
		}
	}
	return false
}

func (m *tuiModel) expandRecursively(emailID email.ID) {
	target := findThreadNode(email.BuildThreadTree(m.rawEmails), emailID)
	if target == nil {
		return
	}
	var markExpanded func(n *email.ThreadNode)
	markExpanded = func(n *email.ThreadNode) {
		m.expandedThreads[threadNodeKey(n)] = true
		for _, child := range n.Children {
			markExpanded(child)
		}
	}
	markExpanded(target)
}

func (m *tuiModel) expandWholeThreadForEmail(emailID email.ID) {
	if emailID == "" || len(m.rawEmails) == 0 {
		return
	}
	if m.expandedThreads == nil {
		m.expandedThreads = make(map[threadKey]bool)
	}
	roots := email.BuildThreadTree(m.rawEmails)
	var findInTree func(n *email.ThreadNode) bool
	findInTree = func(n *email.ThreadNode) bool {
		if n.Email.ID == emailID {
			return true
		}
		for _, child := range n.Children {
			if findInTree(child) {
				return true
			}
		}
		return false
	}
	var targetRoot *email.ThreadNode
	for _, root := range roots {
		if findInTree(root) {
			targetRoot = root
			break
		}
	}
	if targetRoot == nil {
		return
	}
	var markExpanded func(n *email.ThreadNode)
	markExpanded = func(n *email.ThreadNode) {
		m.expandedThreads[threadNodeKey(n)] = true
		for _, child := range n.Children {
			markExpanded(child)
		}
	}
	markExpanded(targetRoot)
}

func (m *tuiModel) cycleTreeSubtreeExpansion(root *label.TreeNode) {
	if root == nil || len(root.Children) == 0 {
		return
	}

	// Find the shallowest depth of any node with children in root's subtree that is not yet expanded.
	minUnexpandedDepth := -1
	var findShallowest func(n *label.TreeNode, depth int)
	findShallowest = func(n *label.TreeNode, depth int) {
		if len(n.Children) > 0 {
			if !n.Expanded {
				if minUnexpandedDepth == -1 || depth < minUnexpandedDepth {
					minUnexpandedDepth = depth
				}
			}
			for _, child := range n.Children {
				findShallowest(child, depth+1)
			}
		}
	}
	findShallowest(root, 0)

	// If all nodes with children in the subtree are already expanded, fold/collapse the whole subtree.
	if minUnexpandedDepth == -1 {
		var collapseSubtree func(n *label.TreeNode)
		collapseSubtree = func(n *label.TreeNode) {
			n.Expanded = false
			for _, child := range n.Children {
				collapseSubtree(child)
			}
		}
		collapseSubtree(root)
		return
	}

	// Otherwise, expand all nodes with children in the subtree at relative depth <= minUnexpandedDepth.
	var expandToDepth func(n *label.TreeNode, depth int)
	expandToDepth = func(n *label.TreeNode, depth int) {
		if len(n.Children) > 0 {
			if depth <= minUnexpandedDepth {
				n.Expanded = true
			}
			for _, child := range n.Children {
				expandToDepth(child, depth+1)
			}
		}
	}
	expandToDepth(root, 0)
}

// threadKey names a message in the thread-expansion state: by its Message-ID
// header when it has one, so the state survives a refresh that renumbers
// backend IDs, and by its backend ID otherwise. The map used to hold both
// kinds as bare strings, so a header value and a backend ID shared one
// namespace; the struct keeps them apart.
type threadKey struct {
	header email.HeaderID
	id     email.ID
}

func threadKeyOf(em *email.Email) threadKey {
	if em.MessageID != "" {
		return threadKey{header: em.MessageID}
	}
	return threadKey{id: em.ID}
}

func threadNodeKey(n *email.ThreadNode) threadKey { return threadKeyOf(n.Email) }

// findThreadNode finds a message by its backend ID, which is unique within
// the folder being threaded.
func findThreadNode(nodes []*email.ThreadNode, emailID email.ID) *email.ThreadNode {
	for _, n := range nodes {
		if n.Email.ID == emailID {
			return n
		}
		if found := findThreadNode(n.Children, emailID); found != nil {
			return found
		}
	}
	return nil
}

func findShallowestUnexpandedDepth(n *email.ThreadNode, expanded map[threadKey]bool, depth int, minDepth *int) {
	if len(n.Children) == 0 {
		return
	}
	if !expanded[threadNodeKey(n)] {
		if *minDepth == -1 || depth < *minDepth {
			*minDepth = depth
		}
	}
	for _, child := range n.Children {
		findShallowestUnexpandedDepth(child, expanded, depth+1, minDepth)
	}
}

func collapseThreadSubtree(n *email.ThreadNode, expanded map[threadKey]bool) {
	delete(expanded, threadNodeKey(n))
	for _, child := range n.Children {
		collapseThreadSubtree(child, expanded)
	}
}

func expandThreadSubtreeToDepth(n *email.ThreadNode, expanded map[threadKey]bool, depth, maxDepth int) {
	if len(n.Children) == 0 {
		return
	}
	if depth <= maxDepth {
		expanded[threadNodeKey(n)] = true
	}
	for _, child := range n.Children {
		expandThreadSubtreeToDepth(child, expanded, depth+1, maxDepth)
	}
}

// threadExpandBudget is how many rows one Tab may open below the current
// message: enough to show a conversation's shape at once, not enough to push
// the rest of the folder off the screen.
const threadExpandBudget = 10

// visibleThreadRows counts the rows the expansion state shows below n.
func visibleThreadRows(n *email.ThreadNode, expanded map[threadKey]bool) int {
	if !expanded[threadNodeKey(n)] {
		return 0
	}
	rows := 0
	for _, child := range n.Children {
		rows += 1 + visibleThreadRows(child, expanded)
	}
	return rows
}

// expandThreadBreadthFirst opens the replies under target one message at a
// time in breadth-first order, nearest replies first, and stops before the
// message whose replies would show more than budget rows below target. It
// reports whether it opened anything.
func expandThreadBreadthFirst(target *email.ThreadNode, expanded map[threadKey]bool, budget int) bool {
	opened := false
	queue := []*email.ThreadNode{target}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if len(n.Children) == 0 {
			continue
		}
		key := threadNodeKey(n)
		if !expanded[key] {
			expanded[key] = true
			if visibleThreadRows(target, expanded) > budget {
				delete(expanded, key)
				return opened
			}
			opened = true
		}
		queue = append(queue, n.Children...)
	}
	return opened
}

// cycleEmailThreadSubtreeExpansion is what Tab does on a message with
// replies: it opens the subtree breadth-first within threadExpandBudget
// rows; when the next message alone would exceed the budget it opens one
// more level across the subtree instead, so a wide thread still unfolds;
// and once every reply is showing it folds the subtree. The view then
// scrolls so that as much of the opened subtree as fits is on screen.
func (m *tuiModel) cycleEmailThreadSubtreeExpansion(emailID email.ID) {
	roots := email.BuildThreadTree(m.rawEmails)
	target := findThreadNode(roots, emailID)
	if target == nil || len(target.Children) == 0 {
		return
	}

	minUnexpandedDepth := -1
	findShallowestUnexpandedDepth(target, m.expandedThreads, 0, &minUnexpandedDepth)

	switch {
	case minUnexpandedDepth == -1:
		collapseThreadSubtree(target, m.expandedThreads)
	case !expandThreadBreadthFirst(target, m.expandedThreads, threadExpandBudget):
		expandThreadSubtreeToDepth(target, m.expandedThreads, 0, minUnexpandedDepth)
	}

	m.rebuildVisibleEmails()
	for i, e := range m.emails {
		if e.ID == emailID {
			m.eIdx = i
			break
		}
	}
	m.revealThreadBelow()
}

// revealThreadBelow scrolls the index so that as much as possible of the
// subtree under the current message is on screen, moving the current message
// up to the top line when the subtree needs the whole page. It never scrolls
// down past the current message.
func (m *tuiModel) revealThreadBelow() {
	if m.eIdx < 0 || m.eIdx >= len(m.emails) {
		return
	}
	depth := m.emails[m.eIdx].ThreadDepth
	last := m.eIdx
	for last+1 < len(m.emails) && m.emails[last+1].ThreadDepth > depth {
		last++
	}
	want := min(m.eIdx, last-m.indexLines()+1)
	if want > m.indexStart {
		m.indexStart = want
	}
}

// markOwnMail marks every message whose sender is one of the user's own
// addresses (my_emails and the accounts' user_email) as authored, wherever it
// is filed: its row then shows the recipient, as a Sent folder's do, and a
// reply to it goes to that recipient rather than back to the user. It only
// ever sets the mark; a row a Sent folder marked keeps it.
func (m *tuiModel) markOwnMail() {
	own := m.cfg.OwnAddresses()
	if len(own) == 0 {
		return
	}
	for i := range m.rawEmails {
		e := &m.rawEmails[i]
		if e.IsAuthored || e.FromRaw == "" {
			continue
		}
		if _, addr := email.ParseSenderInfo(e.FromRaw); own[addr] {
			e.IsAuthored = true
		}
	}
}
