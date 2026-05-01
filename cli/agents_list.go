package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

type (
	openSelectedChatMsg struct {
		chatID uuid.UUID
	}
	openDraftChatMsg struct{}
	refreshChatsMsg  struct{}
)

type chatDisplayRow struct {
	chat       codersdk.Chat
	depth      int
	isSubagent bool
	childCount int
	isExpanded bool
}

type chatListModel struct {
	styles    tuiStyles
	chats     []codersdk.Chat
	expanded  map[uuid.UUID]bool
	cursor    int
	offset    int
	loading   bool
	err       error
	search    textinput.Model
	searching bool
	spinner   spinner.Model
	width     int
	height    int
}

func newChatListModel(styles tuiStyles) chatListModel {
	search := textinput.New()
	search.Placeholder = "Search chats..."
	search.Prompt = "/ "

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.dimmedText

	return chatListModel{
		styles:   styles,
		expanded: make(map[uuid.UUID]bool),
		loading:  true,
		search:   search,
		spinner:  s,
	}
}

func (m chatListModel) searchQuery() string {
	return strings.TrimSpace(strings.ToLower(m.search.Value()))
}

func (m chatListModel) filteredChats() []codersdk.Chat {
	query := m.searchQuery()
	if query == "" {
		return m.chats
	}

	filtered := make([]codersdk.Chat, 0, len(m.chats))
	for _, chat := range m.chats {
		if strings.Contains(strings.ToLower(chat.Title), query) || strings.Contains(strings.ToLower(chat.ID.String()), query) {
			filtered = append(filtered, chat)
			continue
		}
		if chat.LastError != nil && strings.Contains(strings.ToLower(*chat.LastError), query) {
			filtered = append(filtered, chat)
		}
	}

	return filtered
}

func (m chatListModel) displayRows() []chatDisplayRow {
	filtered := m.filteredChats()
	if len(filtered) == 0 {
		return nil
	}

	queryActive := m.searchQuery() != ""
	chatsByID := make(map[uuid.UUID]codersdk.Chat, len(m.chats))
	included := make(map[uuid.UUID]struct{}, len(filtered))
	for _, chat := range m.chats {
		chatsByID[chat.ID] = chat
	}
	for _, chat := range filtered {
		included[chat.ID] = struct{}{}
		if !queryActive {
			continue
		}
		for parentID := chat.ParentChatID; parentID != nil; {
			parent, ok := chatsByID[*parentID]
			if !ok {
				break
			}
			included[parent.ID] = struct{}{}
			parentID = parent.ParentChatID
		}
	}

	childrenOf := make(map[uuid.UUID][]codersdk.Chat)
	roots := make([]codersdk.Chat, 0, len(included))
	for _, chat := range m.chats {
		if _, ok := included[chat.ID]; !ok {
			continue
		}
		if chat.ParentChatID == nil {
			roots = append(roots, chat)
			continue
		}
		if _, ok := included[*chat.ParentChatID]; ok {
			childrenOf[*chat.ParentChatID] = append(childrenOf[*chat.ParentChatID], chat)
		}
	}

	rows := make([]chatDisplayRow, 0, len(included))
	var appendRows func(codersdk.Chat, int)
	appendRows = func(chat codersdk.Chat, depth int) {
		children := childrenOf[chat.ID]
		isExpanded := m.expanded[chat.ID]
		if queryActive && len(children) > 0 {
			isExpanded = true
		}

		rows = append(rows, chatDisplayRow{
			chat:       chat,
			depth:      depth,
			isSubagent: depth > 0,
			childCount: len(children),
			isExpanded: isExpanded,
		})
		if !isExpanded {
			return
		}
		for _, child := range children {
			appendRows(child, depth+1)
		}
	}

	for _, root := range roots {
		appendRows(root, 0)
	}

	return rows
}

func (m chatListModel) selectedRow() (chatDisplayRow, bool) {
	rows := m.displayRows()
	if len(rows) == 0 || m.cursor < 0 || m.cursor >= len(rows) {
		return chatDisplayRow{}, false
	}
	return rows[m.cursor], true
}

func (m *chatListModel) moveCursorToChat(chatID uuid.UUID) {
	rows := m.displayRows()
	for i, row := range rows {
		if row.chat.ID == chatID {
			m.cursor = i
			return
		}
	}
}

type chatExpansionIntent int

const (
	chatExpansionToggle chatExpansionIntent = iota
	chatExpansionExpand
	chatExpansionCollapse
)

func (m *chatListModel) updateSelectedRowExpansion(intent chatExpansionIntent) bool {
	row, ok := m.selectedRow()
	if !ok {
		return false
	}
	if row.childCount == 0 {
		if intent == chatExpansionExpand || row.chat.ParentChatID == nil {
			return false
		}
		parentID := *row.chat.ParentChatID
		m.expanded[parentID] = false
		m.moveCursorToChat(parentID)
		return true
	}

	switch intent {
	case chatExpansionExpand:
		if row.isExpanded {
			return false
		}
		m.expanded[row.chat.ID] = true
	case chatExpansionCollapse:
		if row.isExpanded {
			m.expanded[row.chat.ID] = false
			return true
		}
		if row.chat.ParentChatID == nil || !m.expanded[*row.chat.ParentChatID] {
			return false
		}
		parentID := *row.chat.ParentChatID
		m.expanded[parentID] = false
		m.moveCursorToChat(parentID)
		return true
	case chatExpansionToggle:
		if row.isExpanded && !m.expanded[row.chat.ID] {
			return false
		}
		m.expanded[row.chat.ID] = !row.isExpanded
	default:
		return false
	}

	return true
}

func (m chatListModel) selectedChat() *codersdk.Chat {
	row, ok := m.selectedRow()
	if !ok {
		return nil
	}
	return &row.chat
}

func (m *chatListModel) normalizeCursor() {
	total := len(m.displayRows())
	if total == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	m.cursor = min(max(m.cursor, 0), total-1)
	m.offset, _ = m.visibleWindow(total)
}

func (m chatListModel) visibleChatCount() int {
	overhead := 3
	if m.searching {
		overhead += 2
	}

	visibleCount := m.height - overhead
	if visibleCount < 3 {
		visibleCount = 3
	}
	return visibleCount
}

func (m chatListModel) visibleWindow(total int) (start int, end int) {
	if total == 0 {
		return 0, 0
	}

	visibleCount := m.visibleChatCount()
	maxOffset := max(total-visibleCount, 0)
	cursor := min(max(m.cursor, 0), total-1)
	start = min(max(min(max(m.offset, 0), maxOffset), cursor-visibleCount+1), cursor)
	end = min(start+visibleCount, total)
	return start, end
}

func (m chatListModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m chatListModel) Update(msg tea.Msg) (chatListModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.normalizeCursor()
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case chatsListedMsg:
		m.chats = msg.chats
		m.err = msg.err
		m.loading = false
		m.normalizeCursor()
		return m, nil

	case tea.KeyMsg:
		key := msg.String()
		if m.searching {
			switch key {
			case "esc":
				if m.search.Value() != "" {
					m.search.SetValue("")
				}
				m.search.Blur()
				m.searching = false
				m.normalizeCursor()
				return m, nil
			case "enter":
				m.search.Blur()
				m.searching = false
				m.normalizeCursor()
				return m, nil
			default:
				m.search, cmd = m.search.Update(msg)
				m.normalizeCursor()
				m.offset = 0
				return m, cmd
			}
		}

		navigationHandled, normalizeNavigation := true, true
		switch key {
		case "/", "ctrl+f":
			m.searching = true
			m.search.Focus()
		case "up", "k":
			m.cursor--
		case "down", "j":
			m.cursor++
		case "right", "l":
			normalizeNavigation = m.updateSelectedRowExpansion(chatExpansionExpand)
		case "left", "h":
			normalizeNavigation = m.updateSelectedRowExpansion(chatExpansionCollapse)
		case "x":
			normalizeNavigation = m.updateSelectedRowExpansion(chatExpansionToggle)
		default:
			navigationHandled = false
		}
		if navigationHandled {
			if normalizeNavigation {
				m.normalizeCursor()
			}
			return m, nil
		}

		switch key {
		case "enter":
			selected := m.selectedChat()
			if selected == nil {
				return m, nil
			}
			return m, func() tea.Msg {
				return openSelectedChatMsg{chatID: selected.ID}
			}
		case "n":
			return m, func() tea.Msg {
				return openDraftChatMsg{}
			}
		case "r":
			m.loading = true
			m.err = nil
			return m, func() tea.Msg {
				return refreshChatsMsg{}
			}
		case "q":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m chatListModel) View() string {
	if m.loading {
		return m.spinner.View() + " Loading chats…"
	}

	if m.err != nil {
		return m.styles.errorText.Render(m.err.Error()) + "\n" + m.styles.helpText.Render("Press r to retry")
	}

	rows := m.displayRows()
	lines := make([]string, 0, len(rows)+3)
	if m.searching {
		lines = append(lines, m.styles.searchInput.Render(m.search.View()))
	}

	if len(rows) == 0 {
		if strings.TrimSpace(m.search.Value()) != "" {
			lines = append(lines, m.styles.dimmedText.Render("No matches."))
		} else {
			lines = append(lines, m.styles.dimmedText.Render("No chats yet. Press n to start a new chat."))
		}
		help := fitHelpText(
			m.width,
			"/: search • n: new chat • r: refresh • q: quit",
			"/ search • n new • r refresh • q quit",
			"/ • n • r • q",
		)
		lines = append(lines, m.styles.helpText.Render(help))
		return strings.Join(lines, "\n")
	}

	statusWidth := 12
	start, end := m.visibleWindow(len(rows))
	for i := start; i < end; i++ {
		row := rows[i]
		rowPrefix := "  "
		rowStyle := m.styles.normalItem
		if i == m.cursor {
			rowPrefix = "> "
			rowStyle = m.styles.selectedItem
		}
		if row.depth > 0 {
			rowPrefix += strings.Repeat("  ", row.depth)
		}
		if row.childCount > 0 {
			if row.isExpanded {
				rowPrefix += "▼ "
			} else {
				rowPrefix += "▶ "
			}
		}

		extraText := ""
		extra := ""
		if row.childCount > 0 {
			extraText = fmt.Sprintf(" (%d subagents)", row.childCount)
			extra = m.styles.dimmedText.Render(extraText)
		}

		titleWidth := max(m.width-statusWidth-18-len(rowPrefix)-len(extraText), 20)
		title := m.styles.truncate(sanitizeTerminalRenderableText(row.chat.Title), titleWidth)
		status := m.styles.statusColor(row.chat.Status).Render(string(row.chat.Status))
		rowText := fmt.Sprintf("%s%s %s %s%s", rowPrefix, rowStyle.Render(title), status, m.styles.dimmedText.Render(timeAgo(row.chat.UpdatedAt)), extra)
		lines = append(lines, rowText)

		if row.chat.Status == codersdk.ChatStatusError && row.chat.LastError != nil {
			errWidth := max(m.width-4, 20)
			errPrefix := "    "
			if row.depth > 0 {
				errPrefix += strings.Repeat("  ", row.depth)
			}
			lines = append(lines, errPrefix+m.styles.dimmedText.Render(m.styles.truncate(sanitizeTerminalRenderableText(*row.chat.LastError), errWidth)))
		}
	}

	lines = append(lines, "")
	help := fitHelpText(
		m.width,
		"↑/k: up • ↓/j: down • →/l: expand • ←/h: collapse • x: toggle • enter: open • /: search • n: new chat • r: refresh • q: quit",
		"↑/k up • ↓/j down • →/l expand • ←/h collapse • x toggle • ↵ open • / search • n new • q quit",
		"↑↓ nav • →← fold • x toggle • ↵ open • / search • n new • q quit",
		"↑↓ • →← • x • ↵ • / • n • q",
	)
	lines = append(lines, m.styles.helpText.Render(help))
	return strings.Join(lines, "\n")
}

func timeAgo(t time.Time) string {
	elapsed := time.Since(t)
	if elapsed < time.Minute {
		return "just now"
	}
	if elapsed < time.Hour {
		return fmt.Sprintf("%dm ago", int(elapsed/time.Minute))
	}
	if elapsed < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(elapsed/time.Hour))
	}
	return fmt.Sprintf("%dd ago", int(elapsed/(24*time.Hour)))
}
