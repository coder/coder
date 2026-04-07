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

type openSelectedChatMsg struct {
	chatID uuid.UUID
}

type openDraftChatMsg struct{}

type refreshChatsMsg struct{}

type chatDisplayRow struct {
	chat       codersdk.Chat
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

	matched := make(map[uuid.UUID]struct{}, len(filtered))
	childrenOf := make(map[uuid.UUID][]codersdk.Chat)
	for _, chat := range filtered {
		matched[chat.ID] = struct{}{}
		if chat.ParentChatID != nil {
			childrenOf[*chat.ParentChatID] = append(childrenOf[*chat.ParentChatID], chat)
		}
	}

	roots := make([]codersdk.Chat, 0, len(filtered))
	for _, chat := range m.chats {
		if chat.ParentChatID != nil {
			continue
		}
		if _, ok := matched[chat.ID]; ok {
			roots = append(roots, chat)
			continue
		}
		if len(childrenOf[chat.ID]) > 0 {
			roots = append(roots, chat)
		}
	}

	rows := make([]chatDisplayRow, 0, len(filtered))
	queryActive := m.searchQuery() != ""
	for _, root := range roots {
		children := childrenOf[root.ID]
		isExpanded := m.expanded[root.ID]
		if queryActive && len(children) > 0 {
			isExpanded = true
		}

		rows = append(rows, chatDisplayRow{
			chat:       root,
			childCount: len(children),
			isExpanded: isExpanded,
		})
		if !isExpanded {
			continue
		}
		for _, child := range children {
			rows = append(rows, chatDisplayRow{
				chat:       child,
				isSubagent: true,
			})
		}
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

func (m *chatListModel) expandSelectedRow() bool {
	row, ok := m.selectedRow()
	if !ok || row.isSubagent || row.childCount == 0 || row.isExpanded {
		return false
	}
	m.expanded[row.chat.ID] = true
	return true
}

func (m *chatListModel) collapseSelectedRow() bool {
	row, ok := m.selectedRow()
	if !ok {
		return false
	}
	if row.isSubagent {
		if row.chat.ParentChatID == nil {
			return false
		}
		parentID := *row.chat.ParentChatID
		m.expanded[parentID] = false
		m.moveCursorToChat(parentID)
		return true
	}
	if row.childCount == 0 || !m.expanded[row.chat.ID] {
		return false
	}
	m.expanded[row.chat.ID] = false
	return true
}

func (m *chatListModel) toggleSelectedRowExpansion() bool {
	row, ok := m.selectedRow()
	if !ok {
		return false
	}
	if row.isSubagent {
		return m.collapseSelectedRow()
	}
	if row.childCount == 0 {
		return false
	}
	if row.isExpanded {
		return m.collapseSelectedRow()
	}
	return m.expandSelectedRow()
}

func (m chatListModel) selectedChat() *codersdk.Chat {
	row, ok := m.selectedRow()
	if !ok {
		return nil
	}
	return &row.chat
}

func (m *chatListModel) clampCursor() {
	rows := m.displayRows()
	if len(rows) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(rows) {
		m.cursor = len(rows) - 1
	}
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
	start = max(m.offset, 0)
	maxOffset := max(total-visibleCount, 0)
	if start > maxOffset {
		start = maxOffset
	}
	if m.cursor < start {
		start = m.cursor
	}
	if m.cursor >= start+visibleCount {
		start = m.cursor - visibleCount + 1
	}
	if start < 0 {
		start = 0
	}
	if start > maxOffset {
		start = maxOffset
	}
	end = min(start+visibleCount, total)
	return start, end
}

func (m *chatListModel) ensureCursorVisible() {
	offset, _ := m.visibleWindow(len(m.displayRows()))
	m.offset = offset
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
		m.ensureCursorVisible()
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
		m.clampCursor()
		m.ensureCursorVisible()
		return m, nil

	case tea.KeyMsg:
		if m.searching {
			switch msg.String() {
			case "esc":
				if m.search.Value() != "" {
					m.search.SetValue("")
					m.clampCursor()
					m.offset = 0
					return m, nil
				}
				m.search.Blur()
				m.searching = false
				m.ensureCursorVisible()
				return m, nil
			case "enter":
				m.search.Blur()
				m.searching = false
				m.ensureCursorVisible()
				return m, nil
			default:
				m.search, cmd = m.search.Update(msg)
				m.clampCursor()
				m.offset = 0
				return m, cmd
			}
		}

		switch msg.String() {
		case "/", "ctrl+f":
			m.searching = true
			m.search.Focus()
			m.ensureCursorVisible()
			return m, nil
		case "up", "k":
			m.cursor--
			m.clampCursor()
			m.ensureCursorVisible()
			return m, nil
		case "down", "j":
			m.cursor++
			m.clampCursor()
			m.ensureCursorVisible()
			return m, nil
		case "right", "l":
			if m.expandSelectedRow() {
				m.clampCursor()
				m.ensureCursorVisible()
			}
			return m, nil
		case "left", "h":
			if m.collapseSelectedRow() {
				m.clampCursor()
				m.ensureCursorVisible()
			}
			return m, nil
		case "x":
			if m.toggleSelectedRowExpansion() {
				m.clampCursor()
				m.ensureCursorVisible()
			}
			return m, nil
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
		if row.isSubagent {
			rowPrefix += "  "
		} else if row.childCount > 0 {
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
		title := m.styles.truncate(row.chat.Title, titleWidth)
		status := m.styles.statusColor(row.chat.Status).Render(string(row.chat.Status))
		rowText := fmt.Sprintf("%s%s %s %s%s", rowPrefix, rowStyle.Render(title), status, m.styles.dimmedText.Render(timeAgo(row.chat.UpdatedAt)), extra)
		lines = append(lines, rowText)

		if row.chat.Status == codersdk.ChatStatusError && row.chat.LastError != nil {
			errWidth := max(m.width-4, 20)
			errPrefix := "    "
			if row.isSubagent {
				errPrefix += "  "
			}
			lines = append(lines, errPrefix+m.styles.dimmedText.Render(m.styles.truncate(*row.chat.LastError, errWidth)))
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
