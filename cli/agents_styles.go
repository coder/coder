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
	selectedBlock lipgloss.Style
	normalItem    lipgloss.Style
	dimmedText    lipgloss.Style
	errorText     lipgloss.Style
	searchInput   lipgloss.Style
	separator     lipgloss.Style
	helpText      lipgloss.Style
	modeBadgeExec lipgloss.Style
	modeBadgePlan lipgloss.Style
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

func newTUIStyles(renderers ...*lipgloss.Renderer) tuiStyles {
	renderer := lipgloss.DefaultRenderer()
	if len(renderers) > 0 && renderers[0] != nil {
		renderer = renderers[0]
	}

	return tuiStyles{
		title:        renderer.NewStyle().Bold(true),
		subtitle:     renderer.NewStyle().Faint(true),
		statusBar:    renderer.NewStyle(),
		statusBadge:  renderer.NewStyle().Padding(0, 1),
		selectedItem: renderer.NewStyle().Bold(true),
		selectedBlock: renderer.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "63", Dark: "63"}).
			PaddingLeft(1),
		normalItem: renderer.NewStyle(),
		dimmedText: renderer.NewStyle().Faint(true),
		errorText:  renderer.NewStyle().Foreground(lipgloss.Color("1")),
		searchInput: renderer.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true),
		separator:     renderer.NewStyle().Faint(true),
		helpText:      renderer.NewStyle().Faint(true),
		modeBadgeExec: renderer.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "22", Dark: "42"}),
		modeBadgePlan: renderer.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "214"}),
		userMessage:   renderer.NewStyle().Bold(true).Foreground(lipgloss.Color("6")),
		assistantMsg:  renderer.NewStyle(),
		reasoning:     renderer.NewStyle().Faint(true).Italic(true),
		toolCallStyle: renderer.NewStyle().Foreground(lipgloss.Color("3")),
		toolPending:   renderer.NewStyle().Faint(true).Foreground(lipgloss.Color("3")),
		toolSuccess:   renderer.NewStyle().Foreground(lipgloss.Color("2")),
		compaction:    renderer.NewStyle().Bold(true).Foreground(lipgloss.Color("5")),
		warningText:   renderer.NewStyle().Foreground(lipgloss.Color("3")),
		criticalText:  renderer.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),
		overlayBorder: renderer.NewStyle().BorderStyle(lipgloss.RoundedBorder()).Padding(1),
		composerStyle: renderer.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderTop(true),
	}
}

func (s tuiStyles) statusColor(status codersdk.ChatStatus) lipgloss.Style {
	color := lipgloss.Color("7")
	switch status {
	case codersdk.ChatStatusWaiting, codersdk.ChatStatusPending:
		color = lipgloss.Color("3")
	case codersdk.ChatStatusRunning:
		color = lipgloss.Color("4")
	case codersdk.ChatStatusPaused:
		color = lipgloss.Color("5")
	case codersdk.ChatStatusCompleted:
		color = lipgloss.Color("2")
	case codersdk.ChatStatusError:
		color = lipgloss.Color("1")
	}
	return s.statusBadge.Foreground(color)
}

func (s tuiStyles) truncate(text string, maxWidth int) string {
	_ = s
	return truncateText(text, maxWidth, "", 3)
}
