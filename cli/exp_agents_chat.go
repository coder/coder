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

type spinnerState bool

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
			found := false
			for i, p := range a.parts {
				if p.Type == codersdk.ChatMessagePartTypeToolCall && p.ToolCallID == part.ToolCallID {
					a.parts[i] = part
					found = true
					break
				}
			}
			if !found {
				a.parts = append(a.parts, part)
			}
		}
	default:
		a.parts = append(a.parts, part)
	}
}

func (a streamAccumulator) isPending() bool {
	return a.pending
}

func (a *streamAccumulator) reset() {
	*a = streamAccumulator{}
}

type chatViewModel struct {
	styles              tuiStyles
	chat                *codersdk.Chat
	messages            []codersdk.ChatMessage
	blocks              []chatBlock
	loading             bool
	err                 error
	metadataResolved    bool
	historyResolved     bool
	metadataErr         error
	historyErr          error
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

	ctx                 context.Context
	client              *codersdk.ExperimentalClient
	workspaceID         *uuid.UUID
	modelOverride       *string
	activeChatID        uuid.UUID
	chatGeneration      uint64
	intentionalClose    bool
	creatingChat        bool
	pendingComposerText string

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

func modelOverrideUUID(modelOverride *string) *uuid.UUID {
	if modelOverride == nil {
		return nil
	}

	modelConfigID, err := uuid.Parse(*modelOverride)
	if err != nil {
		return nil
	}
	return &modelConfigID
}

func canonicalChatModelID(provider, model string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + ":" + strings.TrimSpace(model)
}

func normalizeChatModelOverride(modelOverride string) string {
	modelOverride = strings.TrimSpace(modelOverride)
	provider, model, ok := strings.Cut(modelOverride, "/")
	if ok {
		return canonicalChatModelID(provider, model)
	}
	provider, model, ok = strings.Cut(modelOverride, ":")
	if ok {
		return canonicalChatModelID(provider, model)
	}
	return modelOverride
}

func resolveModelConfigID(ctx context.Context, client *codersdk.ExperimentalClient, modelOverride *string) (*uuid.UUID, error) {
	if modelOverride == nil {
		return nil, xerrors.New("model override is required")
	}
	if modelConfigID := modelOverrideUUID(modelOverride); modelConfigID != nil {
		return modelConfigID, nil
	}

	configs, err := client.ListChatModelConfigs(ctx)
	if err != nil {
		return nil, xerrors.Errorf("list chat model configs: %w", err)
	}

	canonicalOverride := normalizeChatModelOverride(*modelOverride)
	for _, config := range configs {
		if canonicalChatModelID(config.Provider, config.Model) != canonicalOverride {
			continue
		}
		modelConfigID := config.ID
		return &modelConfigID, nil
	}

	return nil, xerrors.Errorf("resolve model config ID for %q: no matching enabled model config", *modelOverride)
}

func newChatViewModel(
	ctx context.Context,
	client *codersdk.ExperimentalClient,
	workspaceID *uuid.UUID,
	modelOverride *string,
	styles tuiStyles,
) chatViewModel {
	composer := textinput.New()
	composer.Placeholder = "Type a message..."
	composer.Prompt = "> "
	composer.Focus()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.dimmedText

	model := chatViewModel{
		ctx:              ctx,
		client:           client,
		workspaceID:      workspaceID,
		modelOverride:    modelOverride,
		styles:           styles,
		loading:          false,
		metadataResolved: true,
		historyResolved:  true,
		composerFocused:  true,
		expandedBlocks:   make(map[int]bool),
		autoFollow:       true,
		composer:         composer,
		viewport:         viewport.New(0, 0),
		spinner:          s,
	}
	model.setComposerWidth()
	return model
}

func (m *chatViewModel) setComposerWidth() {
	m.composer.Width = max(10, m.width-4)
}

func (m *chatViewModel) recalcViewportHeight() {
	if m.height <= 0 || m.width <= 0 {
		return
	}

	viewWidth := m.width
	if viewWidth <= 0 {
		viewWidth = 80
	}

	composerView := m.styles.composerStyle.Width(max(10, viewWidth-2)).Render(m.composer.View())
	composerHeight := lipgloss.Height(composerView)

	const nonViewportHeight = 4
	m.viewport.Width = m.width
	m.viewport.Height = max(0, m.height-nonViewportHeight-composerHeight)
}
func (m *chatViewModel) refreshViewport() { m.recalcViewportHeight(); m.syncViewportContent() }

func (m chatViewModel) readyToStartStream() bool {
	return m.metadataResolved && m.historyResolved && m.err == nil && m.chat != nil && m.client != nil && !m.streaming
}

func (m *chatViewModel) finishLoading(wasSpinnerActive bool) (chatViewModel, tea.Cmd) {
	m.err = m.historyErr
	if m.metadataErr != nil {
		m.err = m.metadataErr
	}
	m.loading = !m.metadataResolved || !m.historyResolved
	return m.startStreamIfReady(wasSpinnerActive)
}

// restorePendingComposerIfEmpty restores pending text to the
// composer only when the user has not typed new input since the
// original send was dispatched.
func (m *chatViewModel) restorePendingComposerIfEmpty() {
	if m.pendingComposerText != "" && m.composer.Value() == "" {
		m.composer.SetValue(m.pendingComposerText)
		m.recalcViewportHeight()
	}
}

func (m *chatViewModel) stopStream() {
	m.intentionalClose = true
	if m.streamCloser != nil {
		_ = m.streamCloser.Close()
		m.streaming, m.streamCloser, m.streamEventCh = false, nil, nil
	}
}

// matchesGeneration returns true when the generation embedded in an
// async message matches the current chat session generation. This
// prevents stale results from previous sessions (including drafts)
// from mutating the active view.
func (m chatViewModel) matchesGeneration(gen uint64) bool {
	return m.chatGeneration == gen
}

func (m *chatViewModel) setChat(chat codersdk.Chat) {
	m.chat = &chat
	m.activeChatID = chat.ID
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

func (m chatViewModel) shouldReconnect() bool {
	return m.chat != nil && (m.isInterruptible() || m.chatStatus == codersdk.ChatStatusWaiting)
}
func (m chatViewModel) Init() tea.Cmd { return m.spinner.Tick }
func (m chatViewModel) spinnerActive() bool {
	return m.reconnecting || m.accumulator.pending || m.isInterruptible()
}

func (m chatViewModel) spinnerLabel() string {
	if m.reconnecting {
		return "Reconnecting..."
	}
	return "Thinking..."
}

func (m chatViewModel) startSpinnerIfNeeded(wasSpinnerActive spinnerState, cmd tea.Cmd) tea.Cmd {
	if bool(wasSpinnerActive) || !m.spinnerActive() {
		return cmd
	}
	if cmd == nil {
		return m.spinner.Tick
	}
	return tea.Batch(cmd, m.spinner.Tick)
}

func availableChatModels(catalog codersdk.ChatModelsResponse) []codersdk.ChatModel {
	var models []codersdk.ChatModel
	for _, provider := range catalog.Providers {
		if provider.Available {
			models = append(models, provider.Models...)
		}
	}
	return models
}

// sendMessage trims the composer, builds the content, and dispatches
// a create-chat or send-message command.
func (m chatViewModel) sendMessage() (chatViewModel, tea.Cmd) {
	text := strings.TrimSpace(m.composer.Value())
	if text == "" {
		return m, nil
	}
	if m.loading {
		return m, nil
	}
	if !m.draft && m.chat == nil {
		return m, nil
	}
	if m.draft && m.creatingChat {
		return m, nil
	}
	m.autoFollow = true
	m.pendingComposerText = text
	m.composer.SetValue("")
	(&m).recalcViewportHeight()
	content := []codersdk.ChatInputPart{{
		Type: codersdk.ChatInputPartTypeText,
		Text: text,
	}}

	modelConfigID := modelOverrideUUID(m.modelOverride)

	if m.draft {
		req := codersdk.CreateChatRequest{
			Content:       content,
			WorkspaceID:   m.workspaceID,
			ModelConfigID: modelConfigID,
		}
		m.creatingChat = true
		return m, apiCmd(func() (codersdk.Chat, error) {
			if req.ModelConfigID == nil && m.modelOverride != nil {
				modelConfigID, err := resolveModelConfigID(m.ctx, m.client, m.modelOverride)
				if err != nil {
					return codersdk.Chat{}, err
				}
				req.ModelConfigID = modelConfigID
			}
			return m.client.CreateChat(m.ctx, req)
		}, func(chat codersdk.Chat, err error) tea.Msg {
			return chatCreatedMsg{generation: m.chatGeneration, chatID: chat.ID, chat: chat, err: err}
		})
	}

	req := codersdk.CreateChatMessageRequest{
		Content:       content,
		ModelConfigID: modelConfigID,
	}
	return m, apiCmd(func() (codersdk.CreateChatMessageResponse, error) {
		if req.ModelConfigID == nil && m.modelOverride != nil {
			modelConfigID, err := resolveModelConfigID(m.ctx, m.client, m.modelOverride)
			if err != nil {
				return codersdk.CreateChatMessageResponse{}, err
			}
			req.ModelConfigID = modelConfigID
		}
		return m.client.CreateChatMessage(m.ctx, m.chat.ID, req)
	}, func(resp codersdk.CreateChatMessageResponse, err error) tea.Msg {
		return messageSentMsg{generation: m.chatGeneration, chatID: m.chat.ID, resp: resp, err: err}
	})
}

// startStream opens a streaming connection from the latest known message ID.
func (m chatViewModel) startStream() (chatViewModel, tea.Cmd) {
	if m.chat == nil || m.streaming {
		return m, nil
	}
	m.intentionalClose = false

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
	m.streaming, m.streamCloser, m.streamEventCh, m.reconnecting = true, closer, eventCh, false
	m.syncViewportContent()
	return m, listenToStream(m.activeChatID, m.chatGeneration, m.streamEventCh)
}

func (m chatViewModel) startStreamWithSpinner(wasSpinnerActive bool) (chatViewModel, tea.Cmd) {
	updated, cmd := m.startStream()
	return updated, updated.startSpinnerIfNeeded(spinnerState(wasSpinnerActive), cmd)
}

func (m chatViewModel) startStreamIfReady(wasSpinnerActive bool) (chatViewModel, tea.Cmd) {
	if !m.readyToStartStream() {
		return m, m.startSpinnerIfNeeded(spinnerState(wasSpinnerActive), nil)
	}
	return m.startStreamWithSpinner(wasSpinnerActive)
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
			switch part.Type {
			case codersdk.ChatMessagePartTypeReasoning:
				m.blocks = append(m.blocks, chatBlock{kind: blockReasoning, role: m.accumulator.role, text: part.Text})
			case codersdk.ChatMessagePartTypeToolCall:
				kind := blockToolCall
				if part.ToolName == contextCompactionToolName {
					kind = blockCompaction
				}
				m.blocks = append(m.blocks, chatBlock{
					kind:     kind,
					role:     m.accumulator.role,
					toolName: part.ToolName,
					toolID:   part.ToolCallID,
					args:     compactTranscriptJSON(part.Args),
				})
			case codersdk.ChatMessagePartTypeToolResult:
				kind := blockToolResult
				if part.ToolName == contextCompactionToolName {
					kind = blockCompaction
				}
				m.blocks = append(m.blocks, chatBlock{
					kind:     kind,
					role:     m.accumulator.role,
					toolName: part.ToolName,
					toolID:   part.ToolCallID,
					result:   compactTranscriptJSON(part.Result),
					isError:  part.IsError,
				})
			case codersdk.ChatMessagePartTypeSource:
				title := part.Title
				if title == "" {
					title = part.URL
				}
				m.blocks = append(m.blocks, chatBlock{kind: blockText, role: m.accumulator.role, text: fmt.Sprintf("[Source: %s](%s)", title, part.URL)})
			case codersdk.ChatMessagePartTypeFile:
				m.blocks = append(m.blocks, chatBlock{kind: blockText, role: m.accumulator.role, text: fmt.Sprintf("[File: %s]", part.MediaType)})
			case codersdk.ChatMessagePartTypeFileReference:
				m.blocks = append(m.blocks, chatBlock{kind: blockText, role: m.accumulator.role, text: fmt.Sprintf("[%s L%d-%d]", part.FileName, part.StartLine, part.EndLine)})
			default:
				m.blocks = append(m.blocks, chatBlock{kind: blockText, role: m.accumulator.role, text: part.Text})
			}
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

func (m *chatViewModel) clearPendingStreamAccumulator() {
	m.accumulator.reset()
	m.rebuildBlocks()
}

func (m chatViewModel) handleStreamError(err error, wasSpinnerActive bool) (chatViewModel, tea.Cmd) {
	if !xerrors.Is(err, io.EOF) {
		m.err = err
	}
	m.streaming, m.streamCloser, m.streamEventCh = false, nil, nil
	if m.intentionalClose {
		m.intentionalClose = false
		return m, nil
	}
	if !m.shouldReconnect() {
		return m, nil
	}
	m.clearPendingStreamAccumulator()
	m.reconnecting = true
	m.syncViewportContent()
	updated, cmd := m.startStreamWithSpinner(wasSpinnerActive)
	if updated.streaming {
		updated.err = nil
	}
	return updated, cmd
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

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.setComposerWidth()
		m.refreshViewport()
		return m, nil

	case spinner.TickMsg:
		if !m.spinnerActive() {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.syncViewportContent()
		return m, cmd

	case tea.KeyMsg:
		if msg.String() == "tab" {
			m.composerFocused = !m.composerFocused
			if m.composerFocused {
				m.composer.Focus()
			} else {
				m.composer.Blur()
			}
			m.syncViewportContent()
			return m, nil
		}

		// Shortcut keys take priority over composer input so the parent model
		// can toggle overlays and the chat view can interrupt active chats.
		switch msg.Type {
		case tea.KeyCtrlP:
			return m, func() tea.Msg { return toggleModelPickerMsg{} }
		case tea.KeyCtrlD:
			return m, func() tea.Msg { return toggleDiffDrawerMsg{} }
		case tea.KeyCtrlX:
			if !m.isInterruptible() || m.chat == nil || m.interrupting {
				return m, nil
			}
			m.interrupting = true
			chatID := m.chat.ID
			generation := m.chatGeneration
			ctx := m.ctx
			client := m.client
			return m, apiCmd(func() (codersdk.Chat, error) {
				return client.InterruptChat(ctx, chatID)
			}, func(chat codersdk.Chat, err error) tea.Msg {
				return chatInterruptedMsg{generation: generation, chatID: chatID, chat: chat, err: err}
			})
		}

		if m.composerFocused {
			if msg.Type == tea.KeyEnter {
				return m.sendMessage()
			}
			var cmd tea.Cmd
			m.composer, cmd = m.composer.Update(msg)
			m.refreshViewport()
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
		if !m.matchesGeneration(msg.generation) {
			return m, nil
		}
		m.metadataResolved = true
		if msg.err != nil {
			m.metadataErr = msg.err
		} else {
			m.metadataErr = nil
			m.setChat(msg.chat)
		}
		return m.finishLoading(wasSpinnerActive)

	case chatHistoryMsg:
		if !m.matchesGeneration(msg.generation) {
			return m, nil
		}
		m.historyResolved = true
		if msg.err != nil {
			m.historyErr = msg.err
		} else {
			m.historyErr, m.messages, m.lastUsage = nil, msg.messages, nil
			for i := len(m.messages) - 1; i >= 0; i-- {
				if m.messages[i].Usage != nil {
					m.lastUsage = m.messages[i].Usage
					break
				}
			}
			m.autoFollow = true
			m.rebuildBlocks()
		}
		return m.finishLoading(wasSpinnerActive)

	case chatCreatedMsg:
		if !m.matchesGeneration(msg.generation) {
			return m, nil
		}
		m.creatingChat = false
		if msg.err != nil {
			m.err = msg.err
			m.restorePendingComposerIfEmpty()
			return m, nil
		}
		m.setChat(msg.chat)
		m.draft = false
		m.err, m.pendingComposerText = nil, ""
		return m.startStreamWithSpinner(wasSpinnerActive)

	case messageSentMsg:
		if !m.matchesGeneration(msg.generation) {
			return m, nil
		}
		if msg.err != nil {
			m.err = msg.err
			m.restorePendingComposerIfEmpty()
			return m, nil
		}
		m.err, m.pendingComposerText = nil, ""
		if msg.resp.Message != nil {
			m.addMessageIfNew(*msg.resp.Message)
		}
		if msg.resp.Queued && msg.resp.QueuedMessage != nil {
			m.queuedMessages = []codersdk.ChatQueuedMessage{*msg.resp.QueuedMessage}
		}
		m.rebuildBlocks()
		return m.startStreamIfReady(wasSpinnerActive)

	case chatInterruptedMsg:
		if !m.matchesGeneration(msg.generation) {
			return m, nil
		}
		m.interrupting = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		chat := msg.chat
		m.chat, m.chatStatus = &chat, chat.Status
		m.syncViewportContent()
		return m, m.startSpinnerIfNeeded(spinnerState(wasSpinnerActive), nil)

	case chatStreamEventMsg:
		if !m.matchesGeneration(msg.generation) {
			return m, nil
		}
		if msg.err != nil {
			return m.handleStreamError(msg.err, wasSpinnerActive)
		}
		updated, cmd := m.handleStreamEvent(msg.event)
		return updated, updated.startSpinnerIfNeeded(spinnerState(wasSpinnerActive), cmd)

	case modelsListedMsg:
		if msg.err != nil {
			return m, nil
		}
		m.modelPickerFlat = availableChatModels(msg.catalog)
		if m.modelPickerCursor >= len(m.modelPickerFlat) {
			m.modelPickerCursor = max(len(m.modelPickerFlat)-1, 0)
		}
		return m, nil

	case gitChangesMsg:
		if !m.matchesGeneration(msg.generation) {
			return m, nil
		}
		if msg.err != nil {
			m.diffErr = msg.err
			return m, nil
		}
		m.gitChanges = msg.changes
		return m, nil

	case diffContentsMsg:
		if !m.matchesGeneration(msg.generation) {
			return m, nil
		}
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
		if event.Status != nil && event.ChatID == m.activeChatID {
			m.chatStatus = event.Status.Status
			if m.chat != nil {
				m.chat.Status = event.Status.Status
			}
			m.syncViewportContent()
		}

	case codersdk.ChatStreamEventTypeQueueUpdate:
		m.queuedMessages = event.QueuedMessages
		m.rebuildBlocks()

	case codersdk.ChatStreamEventTypeRetry:
		m.reconnecting = true
		m.syncViewportContent()

	case codersdk.ChatStreamEventTypeError:
		if event.Error != nil {
			m.err = xerrors.Errorf("stream error: %s", event.Error.Message)
		}
	}

	if m.streaming && m.streamEventCh != nil {
		return m, listenToStream(m.activeChatID, m.chatGeneration, m.streamEventCh)
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
		longHelpParts = append(longHelpParts, "ctrl+x: interrupt")
		shortHelpParts = append(shortHelpParts, "ctrl+x")
		compactHelpParts = append(compactHelpParts, "^X")
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
	composerHeight := lipgloss.Height(composerView)
	statusBarHeight := 0
	if statusBar != "" {
		statusBarHeight = lipgloss.Height(statusBar)
	}
	errorBannerHeight := 0
	if errorBanner != "" {
		errorBannerHeight = lipgloss.Height(errorBanner)
	}
	nonViewportHeight := 1 + 1 + statusBarHeight + errorBannerHeight + composerHeight + 1
	availableViewportHeight := max(0, m.height-nonViewportHeight)

	viewportView := m.viewport.View()
	if m.loading && len(m.blocks) == 0 {
		viewportWidth := max(max(m.viewport.Width, viewWidth), 1)
		viewportView = lipgloss.Place(
			viewportWidth,
			max(availableViewportHeight, 1),
			lipgloss.Center,
			lipgloss.Center,
			m.styles.dimmedText.Render("Loading chat..."),
		)
	}
	viewportView = clampLines(viewportView, availableViewportHeight)

	sections := []string{header}
	sections = append(sections, separator, viewportView)
	if statusBar != "" {
		sections = append(sections, statusBar)
	}
	if errorBanner != "" {
		sections = append(sections, errorBanner)
	}
	sections = append(sections, composerView, helpRow)

	return strings.Join(sections, "\n")
}
