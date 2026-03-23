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

type chatListModel struct {
	styles    tuiStyles
	chats     []codersdk.Chat
	cursor    int
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
		styles:  styles,
		loading: true,
		search:  search,
		spinner: s,
	}
}

func (m chatListModel) filteredChats() []codersdk.Chat {
	query := strings.TrimSpace(strings.ToLower(m.search.Value()))
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

func (m chatListModel) selectedChat() *codersdk.Chat {
	filtered := m.filteredChats()
	if len(filtered) == 0 || m.cursor < 0 || m.cursor >= len(filtered) {
		return nil
	}
	return &filtered[m.cursor]
}

func (m *chatListModel) clampCursor() {
	filtered := m.filteredChats()
	if len(filtered) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(filtered) {
		m.cursor = len(filtered) - 1
	}
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
		return m, nil

	case tea.KeyMsg:
		if m.searching {
			switch msg.String() {
			case "esc":
				if m.search.Value() != "" {
					m.search.SetValue("")
					m.clampCursor()
					return m, nil
				}
				m.search.Blur()
				m.searching = false
				return m, nil
			case "enter":
				m.search.Blur()
				m.searching = false
				return m, nil
			default:
				m.search, cmd = m.search.Update(msg)
				m.clampCursor()
				return m, cmd
			}
		}

		switch msg.String() {
		case "/", "ctrl+f":
			m.searching = true
			m.search.Focus()
			return m, nil
		case "up", "k":
			m.cursor--
			m.clampCursor()
			return m, nil
		case "down", "j":
			m.cursor++
			m.clampCursor()
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

	filtered := m.filteredChats()
	lines := make([]string, 0, len(filtered)+3)
	if m.searching {
		lines = append(lines, m.styles.searchInput.Render(m.search.View()))
	}

	if len(filtered) == 0 {
		if strings.TrimSpace(m.search.Value()) != "" {
			lines = append(lines, m.styles.dimmedText.Render("No matches."))
		} else {
			lines = append(lines, m.styles.dimmedText.Render("No chats yet. Press n to start a new chat."))
		}
		lines = append(lines, m.styles.helpText.Render("/: search • n: new chat • r: refresh • q: quit"))
		return strings.Join(lines, "\n")
	}

	statusWidth := 12
	for i, chat := range filtered {
		prefix := "  "
		rowStyle := m.styles.normalItem
		if i == m.cursor {
			prefix = "> "
			rowStyle = m.styles.selectedItem
		}

		titleWidth := max(m.width-statusWidth-18, 20)
		title := m.styles.truncate(chat.Title, titleWidth)
		status := m.styles.statusColor(chat.Status).Render(string(chat.Status))
		row := fmt.Sprintf("%s%s %s %s", prefix, rowStyle.Render(title), status, m.styles.dimmedText.Render(timeAgo(chat.UpdatedAt)))
		lines = append(lines, row)

		if chat.Status == codersdk.ChatStatusError && chat.LastError != nil {
			errWidth := max(m.width-4, 20)
			lines = append(lines, "    "+m.styles.dimmedText.Render(m.styles.truncate(*chat.LastError, errWidth)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, m.styles.helpText.Render("↑/k: up • ↓/j: down • enter: open • /: search • n: new chat • r: refresh • q: quit"))
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
