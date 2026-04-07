package cli

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/coder/coder/v2/codersdk"
)

type tuiStyles struct {
	title         lipgloss.Style
	subtitle      lipgloss.Style
	statusBar     lipgloss.Style
	statusBadge   lipgloss.Style
	selectedItem  lipgloss.Style
	normalItem    lipgloss.Style
	dimmedText    lipgloss.Style
	errorText     lipgloss.Style
	searchInput   lipgloss.Style
	separator     lipgloss.Style
	helpText      lipgloss.Style
	userMessage   lipgloss.Style
	assistantMsg  lipgloss.Style
	reasoning     lipgloss.Style
	toolCallStyle lipgloss.Style
	toolPending   lipgloss.Style
	toolSuccess   lipgloss.Style
	compaction    lipgloss.Style
	warningText   lipgloss.Style
	criticalText  lipgloss.Style
	overlayBorder lipgloss.Style
	composerStyle lipgloss.Style
}

func newTUIStyles() tuiStyles {
	return tuiStyles{
		title:        lipgloss.NewStyle().Bold(true),
		subtitle:     lipgloss.NewStyle().Faint(true),
		statusBar:    lipgloss.NewStyle(),
		statusBadge:  lipgloss.NewStyle().Padding(0, 1),
		selectedItem: lipgloss.NewStyle().Bold(true),
		normalItem:   lipgloss.NewStyle(),
		dimmedText:   lipgloss.NewStyle().Faint(true),
		errorText:    lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		searchInput: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true),
		separator:     lipgloss.NewStyle().Faint(true),
		helpText:      lipgloss.NewStyle().Faint(true),
		userMessage:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")),
		assistantMsg:  lipgloss.NewStyle(),
		reasoning:     lipgloss.NewStyle().Faint(true).Italic(true),
		toolCallStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		toolPending:   lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("3")),
		toolSuccess:   lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		compaction:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")),
		warningText:   lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		criticalText:  lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),
		overlayBorder: lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).Padding(1),
		composerStyle: lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderTop(true),
	}
}

func (s tuiStyles) statusColor(status codersdk.ChatStatus) lipgloss.Style {
	switch status {
	case codersdk.ChatStatusWaiting, codersdk.ChatStatusPending:
		return s.statusBadge.Foreground(lipgloss.Color("3"))
	case codersdk.ChatStatusRunning:
		return s.statusBadge.Foreground(lipgloss.Color("4"))
	case codersdk.ChatStatusPaused:
		return s.statusBadge.Foreground(lipgloss.Color("5"))
	case codersdk.ChatStatusCompleted:
		return s.statusBadge.Foreground(lipgloss.Color("2"))
	case codersdk.ChatStatusError:
		return s.statusBadge.Foreground(lipgloss.Color("1"))
	default:
		return s.statusBadge.Foreground(lipgloss.Color("7"))
	}
}

func (s tuiStyles) truncate(text string, maxWidth int) string {
	_ = s
	if maxWidth <= 0 {
		return ""
	}
	if maxWidth <= 3 {
		return "…"
	}

	runes := []rune(text)
	if len(runes) <= maxWidth {
		return text
	}

	return string(runes[:maxWidth-1]) + "…"
}
