package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
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
	kind           chatBlockKind
	role           codersdk.ChatMessageRole
	text           string
	toolName       string
	toolID         string
	args           string
	result         string
	isError        bool
	collapsedCount int

	cachedRender         string
	cachedWidth          int
	cachedExpanded       bool
	cachedCollapsedCount int
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
	default:
		a.parts = append(a.parts, part)
	}
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
	spinner             spinner.Model
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
	autoFollow      bool
	interrupting    bool

	diffStatus   *codersdk.ChatDiffStatus
	gitChanges   []codersdk.ChatGitChange
	diffContents *codersdk.ChatDiffContents
	diffErr      error

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

	s := spinner.New()
	s.Spinner = spinner.Dot

	model := chatViewModel{
		ctx:             ctx,
		client:          client,
		workspaceID:     workspaceID,
		modelOverride:   modelOverride,
		styles:          styles,
		loading:         true,
		composerFocused: true,
		expandedBlocks:  make(map[int]bool),
		autoFollow:      true,
		composer:        composer,
		viewport:        viewport.New(0, 0),
		spinner:         s,
	}
	model.setComposerWidth()
	return model
}

func (m *chatViewModel) setComposerWidth() {
	m.composer.Width = max(10, m.width-4)
}

func (m *chatViewModel) stopStream() {
	if m.streamCloser != nil {
		_ = m.streamCloser.Close()
		m.streamCloser = nil
		m.streamEventCh = nil
		m.streaming = false
	}
}

func (m *chatViewModel) setChat(chat codersdk.Chat) {
	m.chat = &chat
	m.chatStatus = chat.Status
	m.diffStatus = chat.DiffStatus
	m.gitChanges = nil
	m.diffContents = nil
	m.diffErr = nil
}

func (m chatViewModel) isInterruptible() bool {
	return m.chatStatus == codersdk.ChatStatusPending ||
		m.chatStatus == codersdk.ChatStatusRunning
}

func (m chatViewModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m chatViewModel) spinnerActive() bool {
	return m.reconnecting || m.accumulator.pending || m.isInterruptible()
}

func (m chatViewModel) spinnerLabel() string {
	if m.reconnecting {
		return "Reconnecting..."
	}
	return "Thinking..."
}

// sendMessage trims the composer, builds the content, and dispatches
// a create-chat or send-message command.
func (m chatViewModel) sendMessage() (chatViewModel, tea.Cmd) {
	text := strings.TrimSpace(m.composer.Value())
	if text == "" {
		return m, nil
	}
	m.autoFollow = true
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

	if m.accumulator.pending {
		finalizedToolIDs := make(map[string]struct{}, len(m.blocks))
		for _, block := range m.blocks {
			if block.toolID == "" {
				continue
			}
			finalizedToolIDs[block.toolID] = struct{}{}
		}
		for _, part := range m.accumulator.parts {
			if (part.Type == codersdk.ChatMessagePartTypeToolCall || part.Type == codersdk.ChatMessagePartTypeToolResult) && part.ToolCallID != "" {
				if _, ok := finalizedToolIDs[part.ToolCallID]; ok {
					continue
				}
			}
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
		m.blocks[i].cachedCollapsedCount = oldBlocks[i].cachedCollapsedCount
	}

	if m.selectedBlock >= len(m.blocks) {
		m.selectedBlock = max(len(m.blocks)-1, 0)
	}

	m.syncViewportContent()
}

func (m *chatViewModel) getOrCreateMarkdownRenderer(width int) *glamour.TermRenderer {
	if m.cachedRendererWidth == width && m.cachedRenderer != nil {
		return m.cachedRenderer
	}

	m.cachedRendererWidth = width
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		m.cachedRenderer = nil
		return nil
	}

	m.cachedRenderer = renderer
	return renderer
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

	if m.spinnerActive() {
		indicator := m.spinner.View() + " " + m.spinnerLabel()
		transcript += "\n" + m.styles.dimmedText.Render(indicator)
	}

	if transcript != m.lastTranscript {
		m.lastTranscript = transcript
		m.viewport.SetContent(transcript)
	}
	if m.autoFollow {
		m.viewport.GotoBottom()
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
		kind := blockToolCall
		if part.ToolName == "context_compaction" {
			kind = blockCompaction
		}
		return chatBlock{
			kind:     kind,
			role:     role,
			toolName: part.ToolName,
			toolID:   part.ToolCallID,
			args:     compactTranscriptJSON(part.Args),
		}
	case codersdk.ChatMessagePartTypeToolResult:
		kind := blockToolResult
		if part.ToolName == "context_compaction" {
			kind = blockCompaction
		}
		return chatBlock{
			kind:     kind,
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
	wasSpinnerActive := m.spinnerActive()
	startSpinner := func(updated chatViewModel, cmd tea.Cmd) tea.Cmd {
		if wasSpinnerActive || !updated.spinnerActive() {
			return cmd
		}
		if cmd == nil {
			return updated.spinner.Tick
		}
		return tea.Batch(cmd, updated.spinner.Tick)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		viewportHeight := m.height - 8
		if viewportHeight < 0 {
			viewportHeight = 0
		}
		m.viewport.Width = m.width
		m.viewport.Height = viewportHeight
		(&m).setComposerWidth()
		(&m).syncViewportContent()
		return m, nil

	case spinner.TickMsg:
		if !m.spinnerActive() {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		(&m).syncViewportContent()
		return m, cmd

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
			m.viewport.LineUp(3)
			m.autoFollow = false
		case "down", "j":
			m.viewport.LineDown(3)
			m.autoFollow = m.viewport.AtBottom()
		case "pgup":
			m.viewport.HalfViewUp()
			m.autoFollow = false
		case "pgdown":
			m.viewport.HalfViewDown()
			m.autoFollow = m.viewport.AtBottom()
		case "home":
			m.viewport.GotoTop()
			m.autoFollow = false
		case "end":
			m.viewport.GotoBottom()
			m.autoFollow = true
		default:
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
		m.setChat(chat)
		m.err = nil
		m.loading = false
		if len(m.messages) > 0 && !m.streaming {
			updated, cmd := m.startStream()
			return updated, startSpinner(updated, cmd)
		}
		return m, startSpinner(m, nil)

	case chatHistoryMsg:
		if msg.err != nil {
			m.err = msg.err
			m.loading = false
			return m, nil
		}
		m.err = nil
		m.messages = msg.messages
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Usage != nil {
				m.lastUsage = m.messages[i].Usage
				break
			}
		}
		m.autoFollow = true
		m.rebuildBlocks()
		m.loading = false
		if m.chat != nil && !m.streaming {
			updated, cmd := m.startStream()
			return updated, startSpinner(updated, cmd)
		}
		return m, startSpinner(m, nil)

	case chatCreatedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		chat := msg.chat
		m.setChat(chat)
		m.draft = false
		m.err = nil
		updated, cmd := m.startStream()
		return updated, startSpinner(updated, cmd)

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
		return m, startSpinner(m, nil)

	case chatInterruptedMsg:
		m.interrupting = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		chat := msg.chat
		m.chat = &chat
		m.chatStatus = chat.Status
		(&m).syncViewportContent()
		return m, startSpinner(m, nil)

	case chatStreamEventMsg:
		if msg.err != nil {
			if !xerrors.Is(msg.err, io.EOF) {
				m.err = msg.err
			}
			m.streaming = false
			m.streamCloser = nil
			m.streamEventCh = nil
			if m.isInterruptible() && m.chat != nil {
				m.reconnecting = true
				(&m).syncViewportContent()
				updated, cmd := m.startStream()
				if updated.streaming {
					updated.err = nil
				}
				return updated, startSpinner(updated, cmd)
			}
			return m, nil
		}
		updated, cmd := m.handleStreamEvent(msg.event)
		return updated, startSpinner(updated, cmd)

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
		if msg.err != nil {
			m.diffErr = msg.err
			return m, nil
		}
		m.gitChanges = msg.changes
		return m, nil

	case diffContentsMsg:
		if msg.err != nil {
			m.diffErr = msg.err
			return m, nil
		}
		diff := msg.diff
		m.diffContents = &diff
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
			m.accumulator = streamAccumulator{}
			m.reconnecting = false
			m.rebuildBlocks()
		}

	case codersdk.ChatStreamEventTypeStatus:
		if event.Status != nil {
			m.chatStatus = event.Status.Status
			if m.chat != nil {
				m.chat.Status = event.Status.Status
			}
			(&m).syncViewportContent()
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
	viewWidth := m.width
	if viewWidth <= 0 {
		viewWidth = 80
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

	statusBar := renderStatusBar(
		m.styles,
		m.chat,
		m.chatStatus,
		m.lastUsage,
		len(m.queuedMessages),
		m.interrupting,
		m.reconnecting,
		viewWidth,
	)

	errorBanner := ""
	if m.err != nil {
		errorBanner = m.styles.errorText.Render(m.styles.truncate(strings.ReplaceAll(m.err.Error(), "\n", " "), viewWidth))
	}

	viewportHeight := max(m.viewport.Height, 0)
	if errorBanner != "" {
		viewportHeight = max(viewportHeight-1, 0)
	}

	viewportView := m.viewport.View()
	if m.loading && len(m.blocks) == 0 {
		viewportWidth := max(max(m.viewport.Width, viewWidth), 1)
		viewportHeight = max(viewportHeight, 1)
		viewportView = lipgloss.Place(
			viewportWidth,
			viewportHeight,
			lipgloss.Center,
			lipgloss.Center,
			m.styles.dimmedText.Render("Loading chat..."),
		)
	} else if errorBanner != "" {
		viewportView = clampLines(viewportView, viewportHeight)
	}

	composerView := m.styles.composerStyle.Width(max(10, viewWidth-2)).Render(m.composer.View())

	longHelpParts := []string{"tab: switch focus", "esc: back"}
	shortHelpParts := []string{"tab focus", "esc back"}
	compactHelpParts := []string{"tab", "esc"}
	if m.composerFocused {
		longHelpParts = append(longHelpParts, "enter: send")
		shortHelpParts = append(shortHelpParts, "↵ send")
		compactHelpParts = append(compactHelpParts, "↵")
	} else {
		longHelpParts = append(longHelpParts, "↑↓: scroll", "pgup/pgdn: page", "home/end: jump")
		shortHelpParts = append(shortHelpParts, "↑↓ scroll", "pg page", "home/end")
		compactHelpParts = append(compactHelpParts, "↑↓", "pg", "home/end")
	}
	if m.isInterruptible() {
		longHelpParts = append(longHelpParts, "ctrl+i: interrupt")
		shortHelpParts = append(shortHelpParts, "ctrl+i")
		compactHelpParts = append(compactHelpParts, "^I")
	}
	longHelpParts = append(longHelpParts, "ctrl+p: models", "ctrl+d: diff")
	shortHelpParts = append(shortHelpParts, "ctrl+p", "ctrl+d")
	compactHelpParts = append(compactHelpParts, "^P", "^D")

	helpRow := m.styles.helpText.Render(fitHelpText(
		viewWidth,
		strings.Join(longHelpParts, " | "),
		strings.Join(shortHelpParts, " │ "),
		strings.Join(compactHelpParts, " "),
	))
	separator := m.styles.separator.Render(strings.Repeat("─", max(viewWidth, 1)))
	sections := []string{header}
	if m.chat != nil {
		sections = append(sections, "Status: "+m.styles.statusColor(m.chatStatus).Render(string(m.chatStatus)))
	}

	sections = append(sections, separator, viewportView, separator)
	if statusBar != "" {
		sections = append(sections, statusBar)
	}
	if errorBanner != "" {
		sections = append(sections, errorBanner)
	}
	sections = append(sections, composerView, helpRow)

	return strings.Join(sections, "\n")
}
