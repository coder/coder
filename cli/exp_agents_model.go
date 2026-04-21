package cli

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

type tuiView int

const (
	viewList tuiView = iota
	viewChat
)

type tuiOverlay int

const (
	overlayNone tuiOverlay = iota
	overlayModelPicker
	overlayDiffDrawer
	overlayAskUserQuestion
)

type (
	terminateTUIMsg  struct{}
	expChatsTUIModel struct {
		ctx            context.Context
		client         *codersdk.ExperimentalClient
		styles         tuiStyles
		currentView    tuiView
		overlay        tuiOverlay
		list           chatListModel
		chat           chatViewModel
		initialChatID  *uuid.UUID
		workspaceID    *uuid.UUID
		modelOverride  *string
		organizationID uuid.UUID
		chatGeneration uint64
		catalog        *codersdk.ChatModelsResponse
		quitting       bool
		width          int
		height         int
	}
)

func newExpChatsTUIModel(
	ctx context.Context,
	client *codersdk.ExperimentalClient,
	initialChatID *uuid.UUID,
	workspaceID *uuid.UUID,
	modelOverride *string,
	organizationID uuid.UUID,
) expChatsTUIModel {
	styles := newTUIStyles()
	currentView := viewList
	if initialChatID != nil {
		currentView = viewChat
	}
	chat := newChatViewModel(ctx, client, workspaceID, modelOverride, organizationID, styles)
	chatGeneration := uint64(0)
	if initialChatID != nil {
		chat.activeChatID = *initialChatID
		chat.chatGeneration = 1
		chat.loading = true
		chat.metadataResolved = false
		chat.historyResolved = false
		chatGeneration = 1
	}
	return expChatsTUIModel{
		ctx:            ctx,
		client:         client,
		styles:         styles,
		currentView:    currentView,
		overlay:        overlayNone,
		list:           newChatListModel(styles),
		chat:           chat,
		initialChatID:  initialChatID,
		workspaceID:    workspaceID,
		modelOverride:  modelOverride,
		organizationID: organizationID,
		chatGeneration: chatGeneration,
	}
}

// resetChatSession creates a fresh chatViewModel, preserves the
// window dimensions from the previous session, and advances
// the monotonic generation counter so in-flight async messages
// from the old session are ignored.
func (m *expChatsTUIModel) resetChatSession() {
	old := m.chat
	m.chat = newChatViewModel(m.ctx, m.client, m.workspaceID, m.modelOverride, m.organizationID, m.styles)
	m.chat.width = old.width
	m.chat.height = old.height
	m.chat.loading = true
	m.chat.metadataResolved = false
	m.chat.historyResolved = false
	m.chatGeneration++
	m.chat.chatGeneration = m.chatGeneration
}

func (m *expChatsTUIModel) setRenderer(renderer *lipgloss.Renderer) {
	styles := newTUIStyles(renderer)
	m.styles = styles
	m.list.styles = styles
	m.list.spinner.Style = styles.dimmedText
	m.chat.styles = styles
	m.chat.spinner.Style = styles.dimmedText
}

func (m expChatsTUIModel) Init() tea.Cmd {
	if m.initialChatID != nil {
		m.chat.activeChatID = *m.initialChatID
		return tea.Batch(append([]tea.Cmd{m.chat.Init()}, m.loadChatCmd(*m.initialChatID, m.chat.chatGeneration)...)...)
	}
	return tea.Batch(m.loadChatsCmd(), m.list.Init())
}

func (m expChatsTUIModel) loadChatsCmd() tea.Cmd {
	return apiCmd(func() ([]codersdk.Chat, error) { return m.client.ListChats(m.ctx, nil) }, func(chats []codersdk.Chat, err error) tea.Msg { return chatsListedMsg{chats: chats, err: err} })
}

func (m expChatsTUIModel) loadChatCmd(chatID uuid.UUID, generation uint64) []tea.Cmd {
	return []tea.Cmd{apiCmd(func() (codersdk.Chat, error) { return m.client.GetChat(m.ctx, chatID) }, func(chat codersdk.Chat, err error) tea.Msg {
		return chatOpenedMsg{generation: generation, chatID: chatID, chat: chat, err: err}
	}), loadChatHistoryCmd(m.ctx, m.client, chatID, generation)}
}

func (m expChatsTUIModel) childWindowSizeMsg() tea.WindowSizeMsg {
	h := m.height
	if m.currentView == viewList {
		h = max(0, h-1)
	}
	return tea.WindowSizeMsg{Width: m.width, Height: h}
}

func (m *expChatsTUIModel) toggleOverlay(overlay tuiOverlay) bool {
	if m.overlay == overlay {
		m.overlay = overlayNone
		return false
	}
	m.overlay = overlay
	return true
}

func (m *expChatsTUIModel) handleEsc(msg tea.KeyMsg) tea.Cmd {
	if m.currentView == viewList && m.list.searching {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return cmd
	}
	if m.currentView == viewChat {
		m.chatGeneration++
		m.chat.chatGeneration = m.chatGeneration
		m.chat.stopStream()
		m.currentView = viewList
		m.list.loading = true
		return m.loadChatsCmd()
	}
	m.quitting = true
	return tea.Quit
}

func isOverlayCloseKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyEsc || msg.Type == tea.KeyEscape {
		return true
	}

	key := msg.String()
	return key == "esc" || key == "ctrl+["
}

func (m *expChatsTUIModel) handleModelPickerKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "up", "k":
		if m.chat.modelPickerCursor > 0 {
			m.chat.modelPickerCursor--
		}
	case "down", "j":
		if m.chat.modelPickerCursor < len(m.chat.modelPickerFlat)-1 {
			m.chat.modelPickerCursor++
		}
	case "enter":
		if len(m.chat.modelPickerFlat) > 0 && m.chat.modelPickerCursor < len(m.chat.modelPickerFlat) {
			selected := m.chat.modelPickerFlat[m.chat.modelPickerCursor]
			m.chat.modelOverride = &selected.ID
			m.modelOverride = &selected.ID
			m.overlay = overlayNone
		}
	case "ctrl+p", "q":
		m.overlay = overlayNone
	}
	return nil
}

func (m *expChatsTUIModel) handleAskUserQuestionKey(msg tea.KeyMsg) tea.Cmd {
	state := m.chat.pendingAskUserQuestion
	if state == nil || state.Submitting || len(state.Questions) == 0 {
		return nil
	}
	if state.CurrentIndex < 0 || state.CurrentIndex >= len(state.Questions) {
		return nil
	}

	if state.OtherMode {
		switch msg.Type {
		case tea.KeyEsc:
			state.OtherMode = false
			state.OtherInput.Blur()
			return nil
		case tea.KeyEnter:
			answer := strings.TrimSpace(state.OtherInput.Value())
			if answer == "" {
				return nil
			}
			return m.recordAskAnswer(answer, "", true)
		default:
			var cmd tea.Cmd
			state.OtherInput, cmd = state.OtherInput.Update(msg)
			return cmd
		}
	}

	question := state.Questions[state.CurrentIndex]
	optionCount := len(question.Options) + 1
	switch msg.String() {
	case "up", "k":
		state.OptionCursor--
		if state.OptionCursor < 0 {
			state.OptionCursor = optionCount - 1
		}
	case "down", "j":
		state.OptionCursor++
		if state.OptionCursor >= optionCount {
			state.OptionCursor = 0
		}
	case "left", "h":
		if state.CurrentIndex == 0 {
			return nil
		}
		state.CurrentIndex--
		state.OptionCursor = 0
		state.OtherMode = false
		state.OtherInput.Blur()
		state.Error = nil
		if len(state.Answers) > state.CurrentIndex {
			state.Answers = state.Answers[:state.CurrentIndex]
		}
	case "enter":
		state.Error = nil
		if state.OptionCursor < len(question.Options) {
			option := question.Options[state.OptionCursor]
			answer := strings.TrimSpace(option.Value)
			if answer == "" {
				answer = option.Label
			}
			return m.recordAskAnswer(answer, option.Label, false)
		}
		state.OtherMode = true
		state.OtherInput.SetValue("")
		state.OtherInput.Focus()
	}

	return nil
}

func (m *expChatsTUIModel) recordAskAnswer(answer, optionLabel string, freeform bool) tea.Cmd {
	state := m.chat.pendingAskUserQuestion
	if state == nil || len(state.Questions) == 0 {
		return nil
	}
	if state.CurrentIndex < 0 || state.CurrentIndex >= len(state.Questions) {
		return nil
	}

	question := state.Questions[state.CurrentIndex]
	if len(state.Answers) > state.CurrentIndex {
		state.Answers = state.Answers[:state.CurrentIndex]
	}

	state.Answers = append(state.Answers, askQuestionAnswer{
		Header:      question.Header,
		Question:    question.Question,
		Answer:      answer,
		OptionLabel: optionLabel,
		Freeform:    freeform,
	})
	state.OtherMode = false
	state.OtherInput.Blur()
	state.OtherInput.SetValue("")
	state.OptionCursor = 0
	state.Error = nil

	if state.CurrentIndex+1 < len(state.Questions) {
		state.CurrentIndex++
		return nil
	}

	state.Submitting = true
	return submitAskUserQuestionCmd(m.client.Client, m.chat.activeChatID, m.chat.chatGeneration, state)
}

func (m *expChatsTUIModel) openChatCmd(chatID *uuid.UUID) tea.Cmd {
	m.currentView = viewChat
	m.chat.stopStream()
	m.resetChatSession()
	if chatID == nil {
		m.chat.draft = true
		m.chat.loading = false
		m.chat.metadataResolved = true
		m.chat.historyResolved = true
		m.chat, _ = m.chat.Update(m.childWindowSizeMsg())
		return nil
	}
	m.chat.activeChatID = *chatID
	m.chat, _ = m.chat.Update(m.childWindowSizeMsg())
	return tea.Batch(append([]tea.Cmd{m.chat.Init()}, m.loadChatCmd(*chatID, m.chat.chatGeneration)...)...)
}

func (m *expChatsTUIModel) toggleModelPickerCmd() tea.Cmd {
	if !m.toggleOverlay(overlayModelPicker) {
		return nil
	}
	if m.catalog == nil {
		return apiCmd(func() (codersdk.ChatModelsResponse, error) { return m.client.ListChatModels(m.ctx) }, func(catalog codersdk.ChatModelsResponse, err error) tea.Msg {
			return modelsListedMsg{catalog: catalog, err: err}
		})
	}
	if len(m.chat.modelPickerFlat) == 0 {
		m.chat.modelPickerFlat = availableChatModels(*m.catalog)
	}
	return nil
}

func (m *expChatsTUIModel) toggleDiffDrawerCmd() tea.Cmd {
	if m.chat.chat == nil {
		return nil
	}
	if !m.toggleOverlay(overlayDiffDrawer) {
		return nil
	}
	if m.chat.diffContents == nil || m.chat.diffErr != nil {
		m.chat.diffErr = nil
		chatID := m.chat.chat.ID
		generation := m.chat.chatGeneration
		return apiCmd(func() (codersdk.ChatDiffContents, error) { return fetchChatDiffContents(m.ctx, m.client, chatID) }, func(diff codersdk.ChatDiffContents, err error) tea.Msg {
			return diffContentsMsg{generation: generation, chatID: chatID, diff: diff, err: err}
		})
	}
	return nil
}

func (m expChatsTUIModel) updateChild(msg tea.Msg, view tuiView) (expChatsTUIModel, tea.Cmd) {
	var cmd tea.Cmd
	if view == viewChat {
		m.chat, cmd = m.chat.Update(msg)
	} else {
		m.list, cmd = m.list.Update(msg)
	}
	return m, cmd
}

func (m expChatsTUIModel) renderOverlay(title, body string) string {
	return renderOverlayFrame(m.styles, m.width, m.styles.title.Render(title), body, m.styles.helpText.Render("Esc to close"))
}

func (m expChatsTUIModel) diffOverlayView() string {
	switch {
	case m.chat.diffErr != nil:
		return m.renderOverlay("Diff", m.styles.errorText.Render(wrapPreservingNewlines(m.chat.diffErr.Error(), contentWidth(m.width, 6))))
	case m.chat.diffContents != nil:
		return renderDiffDrawer(m.styles, *m.chat.diffContents, m.chat.diffSummary, m.chat.diffStyledBody, m.width, m.height)
	default:
		return m.renderOverlay("Diff", m.styles.dimmedText.Render("Loading diff…"))
	}
}

func padViewHeight(text string, height int) string {
	if height <= 0 {
		return text
	}
	if text == "" {
		return strings.Repeat("\n", max(height-1, 0))
	}
	lineCount := countRenderedLines(text)
	if lineCount >= height {
		return text
	}
	return text + strings.Repeat("\n", height-lineCount)
}

func (m expChatsTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		childMsg := m.childWindowSizeMsg()
		m.list, _ = m.list.Update(childMsg)
		m.chat, _ = m.chat.Update(childMsg)
		return m, nil
	case terminateTUIMsg:
		m.quitting = true
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.quitting = true
			return m, tea.Quit
		}
		// Handle overlays first so their keys do not leak to the underlying
		// view.
		if m.overlay == overlayAskUserQuestion {
			return m, m.handleAskUserQuestionKey(msg)
		}
		if m.overlay == overlayModelPicker {
			if isOverlayCloseKey(msg) {
				m.overlay = overlayNone
				return m, tea.ClearScreen
			}
			cmd := m.handleModelPickerKey(msg)
			if m.overlay == overlayNone {
				return m, tea.Batch(cmd, tea.ClearScreen)
			}
			return m, cmd
		}
		if m.overlay == overlayDiffDrawer {
			if isOverlayCloseKey(msg) {
				m.overlay = overlayNone
				return m, tea.ClearScreen
			}
			return m, nil
		}
		if msg.String() == "esc" {
			return m, m.handleEsc(msg)
		}
	case openSelectedChatMsg:
		return m, m.openChatCmd(&msg.chatID)
	case openDraftChatMsg:
		return m, m.openChatCmd(nil)
	case refreshChatsMsg:
		return m, m.loadChatsCmd()
	case toggleModelPickerMsg:
		return m, m.toggleModelPickerCmd()
	case toggleDiffDrawerMsg:
		return m, m.toggleDiffDrawerCmd()
	case showAskUserQuestionMsg:
		m.chat.pendingAskUserQuestion = msg.state
		m.overlay = overlayAskUserQuestion
		return m.updateChild(msg, viewChat)
	case hideAskUserQuestionMsg:
		if m.overlay == overlayAskUserQuestion {
			m.overlay = overlayNone
		}
		return m.updateChild(msg, viewChat)
	case toolResultsSubmittedMsg:
		if msg.err == nil && m.chat.matchesGeneration(msg.generation) && msg.chatID == m.chat.activeChatID {
			m.chat.pendingAskUserQuestion = nil
			if m.overlay == overlayAskUserQuestion {
				m.overlay = overlayNone
			}
		}
		return m.updateChild(msg, viewChat)
	case chatsListedMsg:
		return m.updateChild(msg, viewList)
	case chatOpenedMsg, chatHistoryMsg, chatStreamEventMsg, messageSentMsg, chatCreatedMsg, chatInterruptedMsg, diffContentsMsg:
		return m.updateChild(msg, viewChat)
	case modelsListedMsg:
		if msg.err != nil {
			m.overlay = overlayNone
		} else {
			catalog := msg.catalog
			m.catalog = &catalog
		}
		return m.updateChild(msg, viewChat)
	}
	return m.updateChild(msg, m.currentView)
}

func (m expChatsTUIModel) View() string {
	if m.quitting {
		return ""
	}

	var base string
	if m.currentView == viewChat {
		base = m.chat.View()
	} else {
		base = m.styles.title.Render("Coder Chats") + "\n" + m.list.View()
	}

	switch m.overlay {
	case overlayAskUserQuestion:
		if m.chat.pendingAskUserQuestion != nil {
			base += "\n" + renderAskUserQuestion(m.styles, m.chat.pendingAskUserQuestion, m.width, m.height)
		}
	case overlayModelPicker:
		if m.catalog == nil {
			base += "\n" + m.renderOverlay("Select Model", m.styles.dimmedText.Render("Loading models..."))
			break
		}
		selectedID := ""
		if m.chat.modelOverride != nil {
			selectedID = *m.chat.modelOverride
		}
		base += "\n" + renderModelPicker(m.styles, *m.catalog, selectedID, m.chat.modelPickerCursor, m.width, m.height)
	case overlayDiffDrawer:
		base += "\n" + m.diffOverlayView()
	}
	return padViewHeight(base, m.height)
}
