package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

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
	parts      []codersdk.ChatMessagePart
	role       codersdk.ChatMessageRole
	pending    bool
	toolDeltas map[string]string
}

func (a *streamAccumulator) applyDelta(mp codersdk.ChatStreamMessagePart) {
	a.pending = true
	a.role = mp.Role
	part := mp.Part

	switch part.Type {
	case codersdk.ChatMessagePartTypeText, codersdk.ChatMessagePartTypeReasoning:
		if len(a.parts) > 0 && a.parts[len(a.parts)-1].Type == part.Type {
			a.parts[len(a.parts)-1].Text += part.Text
		} else {
			a.parts = append(a.parts, part)
		}
	case codersdk.ChatMessagePartTypeToolCall:
		if part.ArgsDelta != "" {
			if a.toolDeltas == nil {
				a.toolDeltas = make(map[string]string)
			}
			a.toolDeltas[part.ToolCallID] += part.ArgsDelta
			found := false
			for i, p := range a.parts {
				if p.Type == codersdk.ChatMessagePartTypeToolCall && p.ToolCallID == part.ToolCallID {
					a.parts[i].Args = json.RawMessage([]byte(a.toolDeltas[part.ToolCallID]))
					found = true
					break
				}
			}
			if !found {
				newPart := part
				newPart.Args = json.RawMessage([]byte(a.toolDeltas[part.ToolCallID]))
				newPart.ArgsDelta = ""
				a.parts = append(a.parts, newPart)
			}
		} else {
			a.parts = append(a.parts, part)
		}
	case codersdk.ChatMessagePartTypeToolResult:
		a.parts = append(a.parts, part)
	default:
		a.parts = append(a.parts, part)
	}
}

func (a *streamAccumulator) reset() {
	a.parts = nil
	a.role = ""
	a.pending = false
	a.toolDeltas = nil
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

	ctx           context.Context
	client        *codersdk.ExperimentalClient
	workspaceID   *uuid.UUID
	modelOverride *uuid.UUID

	streaming     bool
	streamCloser  io.Closer
	streamEventCh <-chan codersdk.ChatStreamEvent
	reconnecting  bool

	chatStatus     codersdk.ChatStatus
	lastUsage      *codersdk.ChatMessageUsage
	queuedMessages []codersdk.ChatQueuedMessage

	composerFocused bool
	selectedBlock   int
	expandedBlocks  map[int]bool
	interrupting    bool

	diffStatus   *codersdk.ChatDiffStatus
	gitChanges   []codersdk.ChatGitChange
	diffContents *codersdk.ChatDiffContents

	modelPickerFlat   []codersdk.ChatModel
	modelPickerCursor int
}

func newChatViewModel(
	ctx context.Context,
	client *codersdk.ExperimentalClient,
	workspaceID *uuid.UUID,
	modelOverride *uuid.UUID,
	styles tuiStyles,
) chatViewModel {
	composer := textinput.New()
	composer.Placeholder = "Type a message..."
	composer.Prompt = "> "
	composer.Focus()

	return chatViewModel{
		ctx:             ctx,
		client:          client,
		workspaceID:     workspaceID,
		modelOverride:   modelOverride,
		styles:          styles,
		loading:         true,
		composerFocused: true,
		expandedBlocks:  make(map[int]bool),
		composer:        composer,
		viewport:        viewport.New(0, 0),
	}
}

func (m *chatViewModel) stopStream() {
	if m.streamCloser != nil {
		_ = m.streamCloser.Close()
		m.streamCloser = nil
		m.streamEventCh = nil
		m.streaming = false
	}
}

func (m chatViewModel) isInterruptible() bool {
	return m.chatStatus == codersdk.ChatStatusPending ||
		m.chatStatus == codersdk.ChatStatusRunning
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
	scratchAccumulator := streamAccumulator{}
	scratchAccumulator.applyDelta(codersdk.ChatStreamMessagePart{
		Part: codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: "stub",
			ArgsDelta:  "{}",
		},
	})
	scratchAccumulator.reset()
	_ = m.diffStatus
	_ = m.isInterruptible()
	_ = renderChatBlocks(m.styles, m.blocks, m.selectedBlock, m.expandedBlocks, m.composerFocused, m.width)
	_ = renderStatusBar(
		m.styles,
		m.chat,
		m.chatStatus,
		m.lastUsage,
		len(m.queuedMessages),
		m.interrupting,
		m.reconnecting,
		m.width,
	)
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
