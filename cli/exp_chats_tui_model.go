package cli

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
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
	ctx           context.Context
	client        *codersdk.ExperimentalClient
	styles        tuiStyles
	currentView   tuiView
	overlay       tuiOverlay
	list          chatListModel
	chat          chatViewModel
	initialChatID *uuid.UUID
	workspaceID   *uuid.UUID
	modelOverride *uuid.UUID
	catalog       *codersdk.ChatModelsResponse
	quitting      bool
	width         int
	height        int
}

func newExpChatsTUIModel(
	ctx context.Context,
	client *codersdk.ExperimentalClient,
	initialChatID *uuid.UUID,
	workspaceID *uuid.UUID,
	modelOverride *uuid.UUID,
) expChatsTUIModel {
	styles := newTUIStyles()
	currentView := viewList
	if initialChatID != nil {
		currentView = viewChat
	}

	return expChatsTUIModel{
		ctx:           ctx,
		client:        client,
		styles:        styles,
		currentView:   currentView,
		overlay:       overlayNone,
		list:          newChatListModel(styles),
		chat:          newChatViewModel(styles),
		initialChatID: initialChatID,
		workspaceID:   workspaceID,
		modelOverride: modelOverride,
	}
}

func (m expChatsTUIModel) Init() tea.Cmd {
	if m.initialChatID != nil {
		return tea.Batch(
			openChatCmd(m.ctx, m.client, *m.initialChatID),
			loadChatHistoryCmd(m.ctx, m.client, *m.initialChatID),
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
		m.list, _ = m.list.Update(msg)
		m.chat, _ = m.chat.Update(msg)
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
			if m.currentView == viewChat {
				m.currentView = viewList
				m.list.loading = true
				return m, listChatsCmd(m.ctx, m.client)
			}
			m.quitting = true
			return m, tea.Quit
		}

	case openSelectedChatMsg:
		m.currentView = viewChat
		m.chat = newChatViewModel(m.styles)
		m.chat.width = m.width
		m.chat.height = m.height
		m.chat, _ = m.chat.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, tea.Batch(
			openChatCmd(m.ctx, m.client, msg.chatID),
			loadChatHistoryCmd(m.ctx, m.client, msg.chatID),
		)

	case openDraftChatMsg:
		m.currentView = viewChat
		m.chat = newChatViewModel(m.styles)
		m.chat.draft = true
		m.chat.loading = false
		m.chat.width = m.width
		m.chat.height = m.height
		m.chat, _ = m.chat.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, nil

	case refreshChatsMsg:
		return m, listChatsCmd(m.ctx, m.client)

	case chatsListedMsg:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd

	case chatOpenedMsg, chatHistoryMsg:
		var cmd tea.Cmd
		m.chat, cmd = m.chat.Update(msg)
		return m, cmd

	case modelsListedMsg:
		if msg.err == nil {
			catalog := msg.catalog
			m.catalog = &catalog
		}
		return m, nil

	case gitChangesMsg, diffContentsMsg:
		return m, nil
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

	return m.styles.title.Render("Coder Chats") + "\n" + body
}
