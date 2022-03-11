package cliui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

type ListItem struct {
	ID          string
	Title       string
	Description string
}

type ListOptions struct {
	Title string
	Items []ListItem
}

func List(cmd *cobra.Command, opts ListOptions) (string, error) {
	items := make([]list.Item, 0)
	for _, item := range opts.Items {
		items = append(items, teaItem{
			id:          item.ID,
			title:       item.Title,
			description: item.Description,
		})
	}
	model := list.New(items, list.NewDefaultDelegate(), 0, 0)
	model.Title = "Select Template"
	model.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		}
	}
	listModel := &listModel{
		opts:  opts,
		model: model,
	}
	program := tea.NewProgram(listModel, tea.WithInput(cmd.InOrStdin()), tea.WithOutput(cmd.OutOrStdout()))
	err := program.Start()
	if err != nil {
		return "", err
	}
	if listModel.selected != nil {
		for _, item := range opts.Items {
			if item.ID == listModel.selected.id {
				return item.ID, nil
			}
		}
	}

	return "", Canceled
}

// teaItem fulfills the "DefaultItem" interface which allows the title
// and description to display!
type teaItem struct {
	id          string
	title       string
	description string
}

func (l teaItem) FilterValue() string { return l.title + "\n" + l.description }
func (l teaItem) Title() string       { return l.title }
func (l teaItem) Description() string { return l.description }

type listModel struct {
	opts     ListOptions
	model    list.Model
	err      string
	selected *teaItem
}

func (m *listModel) Init() tea.Cmd {
	return tea.EnterAltScreen
}

func (m *listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// topGap, rightGap, bottomGap, leftGap := appStyle.GetPadding()
		m.model.SetSize(msg.Width, msg.Height)

	case tea.KeyMsg:
		// Don't match any of the keys below if we're actively filtering.
		if m.model.FilterState() == list.Filtering {
			break
		}
		switch msg.Type {
		case tea.KeyEnter:
			item := m.model.SelectedItem().(teaItem)
			m.selected = &item
			return m, tea.Quit
		}
	}

	// This will also call our delegate's update function.
	newListModel, cmd := m.model.Update(msg)
	m.model = newListModel
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (l *listModel) View() string {
	return l.model.View()
}
