package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/coder/coder/v2/codersdk"
)

type chatBlockKind int

const (
	blockText chatBlockKind = iota
	blockReasoning
	blockToolCall
	blockToolResult
	blockCompaction
)

type chatBlock struct {
	kind     chatBlockKind
	role     codersdk.ChatMessageRole
	text     string
	toolName string
	toolID   string
	args     string
	result   string
	isError  bool
}

type streamAccumulator struct {
	parts   []codersdk.ChatMessagePart
	role    codersdk.ChatMessageRole
	pending bool
}

func (a streamAccumulator) isPending() bool {
	return a.pending
}

type chatViewModel struct {
	styles      tuiStyles
	chat        *codersdk.Chat
	messages    []codersdk.ChatMessage
	blocks      []chatBlock
	loading     bool
	err         error
	draft       bool
	composer    textinput.Model
	viewport    viewport.Model
	accumulator streamAccumulator
	width       int
	height      int
}

func newChatViewModel(styles tuiStyles) chatViewModel {
	composer := textinput.New()
	composer.Placeholder = "Type a message..."
	composer.Prompt = "> "

	return chatViewModel{
		styles:   styles,
		loading:  true,
		composer: composer,
		viewport: viewport.New(0, 0),
	}
}

func (m chatViewModel) Init() tea.Cmd {
	_ = m
	return nil
}

func (m chatViewModel) Update(msg tea.Msg) (chatViewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		viewportHeight := m.height - 4
		if viewportHeight < 0 {
			viewportHeight = 0
		}
		m.viewport.Width = m.width
		m.viewport.Height = viewportHeight
		return m, nil

	case chatOpenedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.loading = false
			return m, nil
		}
		chat := msg.chat
		m.chat = &chat
		m.err = nil
		m.loading = false
		return m, nil

	case chatHistoryMsg:
		m.messages = msg.messages
		m.blocks = messagesToBlocks(msg.messages)
		m.err = msg.err
		if msg.err != nil {
			m.loading = false
		}
		return m, nil

	default:
		return m, nil
	}
}

func (m chatViewModel) stubContent() string {
	for _, block := range m.blocks {
		switch block.kind {
		case blockText, blockReasoning, blockToolCall, blockToolResult, blockCompaction:
		}
		_ = block.role
		_ = block.text
		_ = block.toolName
		_ = block.toolID
		_ = block.args
		_ = block.result
		_ = block.isError
	}
	_ = len(m.accumulator.parts)
	_ = m.accumulator.role
	_ = m.accumulator.isPending()
	return "[Chat content will be rendered here]"
}

func (m chatViewModel) View() string {
	if m.loading {
		return "Loading chat…"
	}

	if m.err != nil {
		return m.styles.errorText.Render(m.err.Error())
	}

	header := "New Chat (draft)"
	if !m.draft && m.chat != nil {
		chatID := m.chat.ID.String()
		shortID := chatID
		if len(chatID) > 8 {
			shortID = chatID[:8]
		}
		header = fmt.Sprintf("%s (%s)", m.chat.Title, shortID)
	}

	statusLine := ""
	if m.chat != nil {
		statusLine = "Status: " + m.styles.statusColor(m.chat.Status).Render(string(m.chat.Status))
	}

	sections := []string{header}
	if statusLine != "" {
		sections = append(sections, statusLine)
	}
	sections = append(
		sections,
		m.styles.separator.Render(strings.Repeat("-", max(m.width, 1))),
		m.styles.dimmedText.Render(m.stubContent()),
		m.styles.separator.Render(strings.Repeat("-", max(m.width, 1))),
		m.styles.helpText.Render("esc: back to list"),
	)

	return strings.Join(sections, "\n")
}
