package cli

import (
	"context"

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
)

type terminateTUIMsg struct{}

type expChatsTUIModel struct {
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
	chatGeneration uint64
	catalog        *codersdk.ChatModelsResponse
	quitting       bool
	width          int
	height         int
}

func newExpChatsTUIModel(
	ctx context.Context,
	client *codersdk.ExperimentalClient,
	initialChatID *uuid.UUID,
	workspaceID *uuid.UUID,
	modelOverride *string,
) expChatsTUIModel {
	styles := newTUIStyles()
	currentView := viewList
	if initialChatID != nil {
		currentView = viewChat
	}

	chat := newChatViewModel(ctx, client, workspaceID, modelOverride, styles)
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
		chatGeneration: chatGeneration,
	}
}

// resetChatSession creates a fresh chatViewModel, preserves the
// window dimensions from the previous session, and advances
// the monotonic generation counter so in-flight async messages
// from the old session are ignored.
func (m *expChatsTUIModel) resetChatSession() {
	old := m.chat
	m.chat = newChatViewModel(m.ctx, m.client, m.workspaceID, m.modelOverride, m.styles)
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
		return tea.Batch(
			m.chat.Init(),
			openChatCmd(m.ctx, m.client, *m.initialChatID, m.chat.chatGeneration),
			loadChatHistoryCmd(m.ctx, m.client, *m.initialChatID, m.chat.chatGeneration),
		)
	}

	return tea.Batch(
		listChatsCmd(m.ctx, m.client),
		m.list.Init(),
	)
}

func (m expChatsTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		childMsg := msg
		childMsg.Height = max(0, msg.Height-1)
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

		if msg.String() == "esc" {
			if m.overlay != overlayNone {
				m.overlay = overlayNone
				return m, nil
			}
			if m.currentView == viewList && m.list.searching {
				var cmd tea.Cmd
				m.list, cmd = m.list.Update(msg)
				return m, cmd
			}
			if m.currentView == viewChat {
				m.chatGeneration++
				m.chat.chatGeneration = m.chatGeneration
				m.chat.stopStream()
				m.currentView = viewList
				m.list.loading = true
				return m, listChatsCmd(m.ctx, m.client)
			}
			m.quitting = true
			return m, tea.Quit
		}

		if m.overlay == overlayModelPicker {
			switch msg.String() {
			case "up", "k":
				if m.chat.modelPickerCursor > 0 {
					m.chat.modelPickerCursor--
				}
				return m, nil
			case "down", "j":
				if m.chat.modelPickerCursor < len(m.chat.modelPickerFlat)-1 {
					m.chat.modelPickerCursor++
				}
				return m, nil
			case "enter":
				if len(m.chat.modelPickerFlat) > 0 && m.chat.modelPickerCursor < len(m.chat.modelPickerFlat) {
					selected := m.chat.modelPickerFlat[m.chat.modelPickerCursor]
					m.chat.modelOverride = &selected.ID
					m.modelOverride = &selected.ID
					m.overlay = overlayNone
				}
				return m, nil
			}
			return m, nil
		}

		if m.overlay == overlayDiffDrawer {
			return m, nil
		}

	case openSelectedChatMsg:
		m.currentView = viewChat
		m.chat.stopStream()
		m.resetChatSession()
		m.chat.activeChatID = msg.chatID
		childMsg := tea.WindowSizeMsg{Width: m.width, Height: max(0, m.height-1)}
		m.chat, _ = m.chat.Update(childMsg)
		return m, tea.Batch(
			m.chat.Init(),
			openChatCmd(m.ctx, m.client, msg.chatID, m.chat.chatGeneration),
			loadChatHistoryCmd(m.ctx, m.client, msg.chatID, m.chat.chatGeneration),
		)

	case openDraftChatMsg:
		m.currentView = viewChat
		m.chat.stopStream()
		m.resetChatSession()
		m.chat.draft = true
		m.chat.loading = false
		childMsg := tea.WindowSizeMsg{Width: m.width, Height: max(0, m.height-1)}
		m.chat, _ = m.chat.Update(childMsg)
		return m, nil

	case refreshChatsMsg:
		return m, listChatsCmd(m.ctx, m.client)

	case toggleModelPickerMsg:
		if m.overlay == overlayModelPicker {
			m.overlay = overlayNone
		} else {
			m.overlay = overlayModelPicker
			if m.catalog == nil {
				return m, listModelsCmd(m.ctx, m.client)
			}
			if len(m.chat.modelPickerFlat) == 0 {
				m.chat.modelPickerFlat = availableChatModels(*m.catalog)
			}
		}
		return m, nil

	case toggleDiffDrawerMsg:
		if m.overlay == overlayDiffDrawer {
			m.overlay = overlayNone
		} else {
			m.overlay = overlayDiffDrawer
			if m.chat.chat != nil && (m.chat.gitChanges == nil || m.chat.diffContents == nil || m.chat.diffErr != nil) {
				m.chat.diffErr = nil
				chatID := m.chat.chat.ID
				return m, tea.Batch(
					loadGitChangesCmd(m.ctx, m.client, chatID, m.chat.chatGeneration),
					loadDiffContentsCmd(m.ctx, m.client, chatID, m.chat.chatGeneration),
				)
			}
		}
		return m, nil

	case chatsListedMsg:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd

	case chatOpenedMsg, chatHistoryMsg:
		var cmd tea.Cmd
		m.chat, cmd = m.chat.Update(msg)
		return m, cmd

	case chatStreamEventMsg, messageSentMsg, chatCreatedMsg, chatInterruptedMsg:
		var cmd tea.Cmd
		m.chat, cmd = m.chat.Update(msg)
		return m, cmd

	case modelsListedMsg:
		if msg.err != nil {
			m.overlay = overlayNone
		} else {
			catalog := msg.catalog
			m.catalog = &catalog
		}
		var cmd tea.Cmd
		m.chat, cmd = m.chat.Update(msg)
		return m, cmd

	case gitChangesMsg, diffContentsMsg:
		var cmd tea.Cmd
		m.chat, cmd = m.chat.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	if m.currentView == viewChat {
		m.chat, cmd = m.chat.Update(msg)
	} else {
		m.list, cmd = m.list.Update(msg)
	}
	return m, cmd
}

func (m expChatsTUIModel) View() string {
	if m.quitting {
		return ""
	}

	body := m.list.View()
	if m.currentView == viewChat {
		body = m.chat.View()
	}

	base := m.styles.title.Render("Coder Chats") + "\n" + body

	switch m.overlay {
	case overlayModelPicker:
		if m.catalog == nil {
			base += "\n" + renderOverlayFrame(
				m.styles,
				m.width,
				m.styles.title.Render("Select Model"),
				m.styles.dimmedText.Render("Loading models..."),
				m.styles.helpText.Render("Esc to close"),
			)
			break
		}
		selectedID := ""
		if m.chat.modelOverride != nil {
			selectedID = *m.chat.modelOverride
		}
		base += "\n" + renderModelPicker(m.styles, *m.catalog, selectedID, m.chat.modelPickerCursor, m.width, m.height)
	case overlayDiffDrawer:
		switch {
		case m.chat.diffErr != nil:
			base += "\n" + renderDiffDrawerError(m.styles, m.chat.diffErr, m.width, m.height)
		case m.chat.diffContents != nil:
			base += "\n" + renderDiffDrawer(m.styles, *m.chat.diffContents, m.chat.gitChanges, m.width, m.height)
		default:
			base += "\n" + renderDiffDrawerLoading(m.styles, m.width, m.height)
		}
	}

	return base
}
