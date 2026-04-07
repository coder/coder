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
	"github.com/charmbracelet/glamour"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

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

	cachedRender   string
	cachedWidth    int
	cachedExpanded bool
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
	styles              tuiStyles
	chat                *codersdk.Chat
	messages            []codersdk.ChatMessage
	blocks              []chatBlock
	loading             bool
	err                 error
	draft               bool
	composer            textinput.Model
	viewport            viewport.Model
	accumulator         streamAccumulator
	width               int
	height              int
	cachedRenderer      *glamour.TermRenderer
	cachedRendererWidth int
	lastTranscript      string

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

// sendMessage trims the composer, builds the content, and dispatches
// a create-chat or send-message command.
func (m chatViewModel) sendMessage() (chatViewModel, tea.Cmd) {
	text := strings.TrimSpace(m.composer.Value())
	if text == "" {
		return m, nil
	}
	m.composer.SetValue("")
	content := []codersdk.ChatInputPart{{
		Type: codersdk.ChatInputPartTypeText,
		Text: text,
	}}

	if m.draft {
		req := codersdk.CreateChatRequest{
			Content:       content,
			WorkspaceID:   m.workspaceID,
			ModelConfigID: m.modelOverride,
		}
		return m, createChatCmd(m.ctx, m.client, req)
	}

	if m.chat == nil {
		return m, nil
	}

	req := codersdk.CreateChatMessageRequest{
		Content:       content,
		ModelConfigID: m.modelOverride,
	}
	return m, sendMessageCmd(m.ctx, m.client, m.chat.ID, req)
}

// startStream opens a streaming connection from the latest known message ID.
func (m chatViewModel) startStream() (chatViewModel, tea.Cmd) {
	if m.chat == nil || m.streaming {
		return m, nil
	}

	var opts *codersdk.StreamChatOptions
	if len(m.messages) > 0 {
		lastID := m.messages[len(m.messages)-1].ID
		opts = &codersdk.StreamChatOptions{AfterID: &lastID}
	}

	eventCh, closer, err := m.client.StreamChat(m.ctx, m.chat.ID, opts)
	if err != nil {
		m.err = err
		return m, nil
	}
	m.streaming = true
	m.streamCloser = closer
	m.streamEventCh = eventCh
	m.reconnecting = false
	(&m).syncViewportContent()
	return m, listenToStream(m.streamEventCh)
}

// rebuildBlocks merges persisted messages + accumulator into renderable blocks.
func (m *chatViewModel) rebuildBlocks() {
	oldBlocks := m.blocks
	m.blocks = messagesToBlocks(m.messages)

	if m.accumulator.isPending() {
		for _, part := range m.accumulator.parts {
			block := partToBlock(part, m.accumulator.role)
			m.blocks = append(m.blocks, block)
		}
	}

	m.blocks = mergeConsecutiveToolBlocks(m.blocks)

	for _, qm := range m.queuedMessages {
		for _, part := range qm.Content {
			if part.Type == codersdk.ChatMessagePartTypeText && part.Text != "" {
				m.blocks = append(m.blocks, chatBlock{
					kind: blockText,
					role: codersdk.ChatMessageRoleUser,
					text: part.Text,
				})
			}
		}
	}

	for i := range m.blocks {
		if i >= len(oldBlocks) || !blockPayloadEqual(m.blocks[i], oldBlocks[i]) {
			continue
		}
		m.blocks[i].cachedRender = oldBlocks[i].cachedRender
		m.blocks[i].cachedWidth = oldBlocks[i].cachedWidth
		m.blocks[i].cachedExpanded = oldBlocks[i].cachedExpanded
	}

	if m.selectedBlock >= len(m.blocks) {
		m.selectedBlock = max(len(m.blocks)-1, 0)
	}

	m.syncViewportContent()
}

func (m *chatViewModel) getOrCreateMarkdownRenderer(width int) *glamour.TermRenderer {
	if m.cachedRenderer != nil && m.cachedRendererWidth == width {
		return m.cachedRenderer
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}

	m.cachedRenderer = renderer
	m.cachedRendererWidth = width
	return m.cachedRenderer
}

func (m *chatViewModel) syncViewportContent() {
	wrapWidth := m.width
	if wrapWidth <= 0 {
		wrapWidth = 80
	}

	transcript := renderChatBlocks(
		m.styles,
		m.blocks,
		m.selectedBlock,
		m.expandedBlocks,
		m.composerFocused,
		m.width,
		m.getOrCreateMarkdownRenderer(wrapWidth),
	)

	if m.accumulator.isPending() || m.reconnecting {
		indicator := "▍"
		if m.reconnecting {
			indicator = "reconnecting…"
		}
		transcript += "\n" + m.styles.dimmedText.Render(indicator)
	}

	if transcript != m.lastTranscript {
		m.lastTranscript = transcript
		m.viewport.SetContent(transcript)
	}
}

func blockPayloadEqual(a, b chatBlock) bool {
	return a.kind == b.kind &&
		a.role == b.role &&
		a.text == b.text &&
		a.toolName == b.toolName &&
		a.toolID == b.toolID &&
		a.args == b.args &&
		a.result == b.result &&
		a.isError == b.isError
}

func partToBlock(part codersdk.ChatMessagePart, role codersdk.ChatMessageRole) chatBlock {
	switch part.Type {
	case codersdk.ChatMessagePartTypeReasoning:
		return chatBlock{kind: blockReasoning, role: role, text: part.Text}
	case codersdk.ChatMessagePartTypeToolCall:
		if part.ToolName == "context_compaction" {
			return chatBlock{
				kind:     blockCompaction,
				role:     role,
				toolName: part.ToolName,
				toolID:   part.ToolCallID,
				args:     compactTranscriptJSON(part.Args),
			}
		}
		return chatBlock{
			kind:     blockToolCall,
			role:     role,
			toolName: part.ToolName,
			toolID:   part.ToolCallID,
			args:     compactTranscriptJSON(part.Args),
		}
	case codersdk.ChatMessagePartTypeToolResult:
		if part.ToolName == "context_compaction" {
			return chatBlock{
				kind:     blockCompaction,
				role:     role,
				toolName: part.ToolName,
				toolID:   part.ToolCallID,
				result:   compactTranscriptJSON(part.Result),
				isError:  part.IsError,
			}
		}
		return chatBlock{
			kind:     blockToolResult,
			role:     role,
			toolName: part.ToolName,
			toolID:   part.ToolCallID,
			result:   compactTranscriptJSON(part.Result),
			isError:  part.IsError,
		}
	case codersdk.ChatMessagePartTypeSource:
		title := part.Title
		if title == "" {
			title = part.URL
		}
		return chatBlock{kind: blockText, role: role, text: fmt.Sprintf("[Source: %s](%s)", title, part.URL)}
	case codersdk.ChatMessagePartTypeFile:
		return chatBlock{kind: blockText, role: role, text: fmt.Sprintf("[File: %s]", part.MediaType)}
	case codersdk.ChatMessagePartTypeFileReference:
		return chatBlock{kind: blockText, role: role, text: fmt.Sprintf("[%s L%d-%d]", part.FileName, part.StartLine, part.EndLine)}
	default:
		return chatBlock{kind: blockText, role: role, text: part.Text}
	}
}

func (m *chatViewModel) addMessageIfNew(msg codersdk.ChatMessage) bool {
	for _, existing := range m.messages {
		if existing.ID == msg.ID {
			return false
		}
	}
	m.messages = append(m.messages, msg)
	return true
}

func (m chatViewModel) Update(msg tea.Msg) (chatViewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		viewportHeight := m.height - 6
		if viewportHeight < 0 {
			viewportHeight = 0
		}
		m.viewport.Width = m.width
		m.viewport.Height = viewportHeight
		(&m).syncViewportContent()
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "tab" {
			m.composerFocused = !m.composerFocused
			if m.composerFocused {
				m.composer.Focus()
			} else {
				m.composer.Blur()
			}
			(&m).syncViewportContent()
			return m, nil
		}

		// Handle the tab string first because Ctrl+I and Tab often collide in
		// terminals.

		// Shortcut keys take priority over composer input so the parent model
		// can toggle overlays and the chat view can interrupt active chats.
		switch msg.Type {
		case tea.KeyCtrlP:
			return m, func() tea.Msg { return toggleModelPickerMsg{} }
		case tea.KeyCtrlD:
			return m, func() tea.Msg { return toggleDiffDrawerMsg{} }
		case tea.KeyCtrlI:
			if m.isInterruptible() && !m.interrupting && m.chat != nil {
				m.interrupting = true
				return m, interruptChatCmd(m.ctx, m.client, m.chat.ID)
			}
			return m, nil
		}

		if m.composerFocused {
			if msg.Type == tea.KeyEnter {
				return m.sendMessage()
			}
			var cmd tea.Cmd
			m.composer, cmd = m.composer.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "up", "k":
			if m.selectedBlock > 0 {
				m.selectedBlock--
			}
			(&m).syncViewportContent()
			return m, nil
		case "down", "j":
			if m.selectedBlock < len(m.blocks)-1 {
				m.selectedBlock++
			}
			(&m).syncViewportContent()
			return m, nil
		case "enter", " ":
			if m.selectedBlock >= 0 && m.selectedBlock < len(m.blocks) {
				m.expandedBlocks[m.selectedBlock] = !m.expandedBlocks[m.selectedBlock]
			}
			(&m).syncViewportContent()
			return m, nil
		}

		return m, nil

	case chatOpenedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.loading = false
			return m, nil
		}
		chat := msg.chat
		m.chat = &chat
		m.chatStatus = chat.Status
		m.diffStatus = chat.DiffStatus
		m.err = nil
		m.loading = false
		if len(m.messages) > 0 && !m.streaming {
			return m.startStream()
		}
		return m, nil

	case chatHistoryMsg:
		if msg.err != nil {
			m.err = msg.err
			m.loading = false
			return m, nil
		}
		m.messages = msg.messages
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Usage != nil {
				m.lastUsage = m.messages[i].Usage
				break
			}
		}
		m.rebuildBlocks()
		m.loading = false
		if m.chat != nil && !m.streaming {
			return m.startStream()
		}
		return m, nil

	case chatCreatedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		chat := msg.chat
		m.chat = &chat
		m.chatStatus = chat.Status
		m.diffStatus = chat.DiffStatus
		m.draft = false
		m.err = nil
		return m.startStream()

	case messageSentMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if msg.resp.Message != nil {
			m.addMessageIfNew(*msg.resp.Message)
		}
		if msg.resp.Queued && msg.resp.QueuedMessage != nil {
			m.queuedMessages = []codersdk.ChatQueuedMessage{*msg.resp.QueuedMessage}
		}
		m.rebuildBlocks()
		return m, nil

	case chatInterruptedMsg:
		m.interrupting = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		chat := msg.chat
		m.chat = &chat
		m.chatStatus = chat.Status
		return m, nil

	case chatStreamEventMsg:
		if msg.err != nil {
			m.streaming = false
			m.streamCloser = nil
			m.streamEventCh = nil
			if m.isInterruptible() && m.chat != nil {
				m.reconnecting = true
				(&m).syncViewportContent()
				return m.startStream()
			}
			return m, nil
		}
		return m.handleStreamEvent(msg.event)

	case modelsListedMsg:
		if msg.err != nil {
			return m, nil
		}
		m.modelPickerFlat = nil
		for _, provider := range msg.catalog.Providers {
			if provider.Available {
				m.modelPickerFlat = append(m.modelPickerFlat, provider.Models...)
			}
		}
		if m.modelPickerCursor >= len(m.modelPickerFlat) {
			m.modelPickerCursor = max(len(m.modelPickerFlat)-1, 0)
		}
		return m, nil

	case gitChangesMsg:
		if msg.err == nil {
			m.gitChanges = msg.changes
		}
		return m, nil

	case diffContentsMsg:
		if msg.err == nil {
			diff := msg.diff
			m.diffContents = &diff
		}
		return m, nil

	default:
		return m, nil
	}
}

func (m chatViewModel) handleStreamEvent(event codersdk.ChatStreamEvent) (chatViewModel, tea.Cmd) {
	switch event.Type {
	case codersdk.ChatStreamEventTypeMessagePart:
		if event.MessagePart != nil {
			m.accumulator.applyDelta(*event.MessagePart)
			m.rebuildBlocks()
		}

	case codersdk.ChatStreamEventTypeMessage:
		if event.Message != nil {
			m.addMessageIfNew(*event.Message)
			if event.Message.Usage != nil {
				m.lastUsage = event.Message.Usage
			}
			m.accumulator.reset()
			m.reconnecting = false
			m.rebuildBlocks()
		}

	case codersdk.ChatStreamEventTypeStatus:
		if event.Status != nil {
			m.chatStatus = event.Status.Status
			if m.chat != nil {
				m.chat.Status = event.Status.Status
			}
		}

	case codersdk.ChatStreamEventTypeQueueUpdate:
		m.queuedMessages = event.QueuedMessages
		m.rebuildBlocks()

	case codersdk.ChatStreamEventTypeRetry:
		m.reconnecting = true
		(&m).syncViewportContent()

	case codersdk.ChatStreamEventTypeError:
		if event.Error != nil {
			m.err = xerrors.Errorf("stream error: %s", event.Error.Message)
		}
	}

	if m.streaming && m.streamEventCh != nil {
		return m, listenToStream(m.streamEventCh)
	}
	return m, nil
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
		statusLine = "Status: " + m.styles.statusColor(m.chatStatus).Render(string(m.chatStatus))
	}

	statusBar := renderStatusBar(
		m.styles,
		m.chat,
		m.chatStatus,
		m.lastUsage,
		len(m.queuedMessages),
		m.interrupting,
		m.reconnecting,
		m.width,
	)

	composerView := m.styles.composerStyle.Render(m.composer.View())

	longHelpParts := []string{"tab: switch focus", "esc: back"}
	shortHelpParts := []string{"tab focus", "esc back"}
	compactHelpParts := []string{"tab", "esc"}
	if m.composerFocused {
		longHelpParts = append(longHelpParts, "enter: send")
		shortHelpParts = append(shortHelpParts, "↵ send")
		compactHelpParts = append(compactHelpParts, "↵")
	} else {
		longHelpParts = append(longHelpParts, "↑↓: navigate", "enter: expand/collapse")
		shortHelpParts = append(shortHelpParts, "↑↓ nav", "↵ toggle")
		compactHelpParts = append(compactHelpParts, "↑↓", "↵")
	}
	if m.isInterruptible() {
		longHelpParts = append(longHelpParts, "ctrl+i: interrupt")
		shortHelpParts = append(shortHelpParts, "ctrl+i")
		compactHelpParts = append(compactHelpParts, "^I")
	}
	longHelpParts = append(longHelpParts, "ctrl+p: models", "ctrl+d: diff")
	shortHelpParts = append(shortHelpParts, "ctrl+p", "ctrl+d")
	compactHelpParts = append(compactHelpParts, "^P", "^D")
	helpRow := fitHelpText(
		m.width,
		strings.Join(longHelpParts, " | "),
		strings.Join(shortHelpParts, " │ "),
		strings.Join(compactHelpParts, " "),
	)
	helpRow = m.styles.helpText.Render(helpRow)

	sections := []string{header}
	if statusLine != "" {
		sections = append(sections, statusLine)
	}
	sections = append(sections,
		m.styles.separator.Render(strings.Repeat("─", max(m.width, 1))),
		m.viewport.View(),
		m.styles.separator.Render(strings.Repeat("─", max(m.width, 1))),
	)
	if statusBar != "" {
		sections = append(sections, statusBar)
	}
	sections = append(sections, composerView, helpRow)

	return strings.Join(sections, "\n")
}
