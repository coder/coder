package cliui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

type SelectOptions struct {
	Options []string
	// Default will be highlighted first if it's a valid option.
	Default    string
	Message    string
	Size       int
	HideSearch bool
}

type RichSelectOptions struct {
	Options    []codersdk.TemplateVersionParameterOption
	Default    string
	Size       int
	HideSearch bool
}

// RichSelect displays a list of user options including name and description.
func RichSelect(inv *serpent.Invocation, richOptions RichSelectOptions) (*codersdk.TemplateVersionParameterOption, error) {
	opts := make([]string, len(richOptions.Options))
	var defaultOpt string
	for i, option := range richOptions.Options {
		line := option.Name
		if len(option.Description) > 0 {
			line += ": " + option.Description
		}
		opts[i] = line

		if option.Value == richOptions.Default {
			defaultOpt = line
		}
	}

	selected, err := Select(inv, SelectOptions{
		Options:    opts,
		Default:    defaultOpt,
		Size:       richOptions.Size,
		HideSearch: richOptions.HideSearch,
	})
	if err != nil {
		return nil, err
	}

	for i, option := range opts {
		if option == selected {
			return &richOptions.Options[i], nil
		}
	}
	return nil, xerrors.Errorf("unknown option selected: %s", selected)
}

// Select displays a list of user options.
func Select(inv *serpent.Invocation, opts SelectOptions) (string, error) {
	initialModel := selectModel{
		search:     textinput.New(),
		hideSearch: opts.HideSearch,
		options:    opts.Options,
		height:     opts.Size,
		message:    opts.Message,
	}

	if initialModel.height == 0 {
		initialModel.height = 5 // TODO: Pick a default?
	}

	initialModel.search.Prompt = ""
	initialModel.search.Focus()

	m, err := tea.NewProgram(
		initialModel,
		tea.WithInput(inv.Stdin),
		tea.WithOutput(inv.Stdout),
	).Run()

	var value string
	if m, ok := m.(selectModel); ok {
		if m.canceled {
			return value, Canceled
		}
		value = m.selected
	}
	return value, err
}

type selectModel struct {
	search     textinput.Model
	options    []string
	cursor     int
	height     int
	message    string
	selected   string
	canceled   bool
	hideSearch bool
}

func (selectModel) Init() tea.Cmd {
	return textinput.Blink
}

//nolint:revive // The linter complains about modifying 'm' but this is typical practice for bubbletea
func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.Type {
		case tea.KeyCtrlC:
			m.canceled = true
			return m, tea.Quit

		case tea.KeyEnter:
			options := m.filteredOptions()
			if len(options) != 0 {
				m.selected = options[m.cursor]
				return m, tea.Quit
			}

		case tea.KeyUp:
			options := m.filteredOptions()
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = len(options) - 1
			}

		case tea.KeyDown:
			options := m.filteredOptions()
			if m.cursor < len(options)-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}
		}
	}

	if !m.hideSearch {
		oldSearch := m.search.Value()
		m.search, cmd = m.search.Update(msg)

		// If the search query has changed then we need to ensure
		// the cursor is still pointing at a valid option.
		if m.search.Value() != oldSearch {
			options := m.filteredOptions()

			if m.cursor > len(options)-1 {
				m.cursor = max(0, len(options)-1)
			}
		}
	}

	return m, cmd
}

func (m selectModel) View() string {
	var s string

	msg := pretty.Sprintf(pretty.Bold(), "? %s", m.message)

	if m.selected == "" {
		if m.hideSearch {
			s += fmt.Sprintf("%s [Use arrows to move]\n", msg)
		} else {
			s += fmt.Sprintf("%s %s[Use arrows to move, type to filter]\n", msg, m.search.View())
		}

		options, start := m.viewableOptions()

		for i, option := range options {
			// Is this the currently selected option?
			style := pretty.Wrap("  ", "")
			if m.cursor == start+i {
				style = pretty.Style{
					pretty.Wrap("> ", ""),
					pretty.FgColor(Green),
				}
			}

			s += pretty.Sprint(style, option)
			s += "\n"
		}
	} else {
		selected := pretty.Sprint(DefaultStyles.Keyword, m.selected)
		s += fmt.Sprintf("%s %s\n", msg, selected)
	}

	return s
}

func (m selectModel) viewableOptions() ([]string, int) {
	options := m.filteredOptions()
	halfHeight := m.height / 2
	bottom := 0
	top := len(options)

	switch {
	case m.cursor <= halfHeight:
		top = min(top, m.height)
	case m.cursor < top-halfHeight:
		bottom = m.cursor - halfHeight
		top = min(top, m.cursor+halfHeight+1)
	default:
		bottom = top - m.height
	}

	return options[bottom:top], bottom
}

func (m selectModel) filteredOptions() []string {
	options := []string{}
	for _, o := range m.options {
		filter := strings.ToLower(m.search.Value())
		option := strings.ToLower(o)

		if strings.Contains(option, filter) {
			options = append(options, o)
		}
	}
	return options
}

type MultiSelectOptions struct {
	Message  string
	Options  []string
	Defaults []string
}

func MultiSelect(inv *serpent.Invocation, opts MultiSelectOptions) ([]string, error) {
	options := make([]*multiSelectOption, len(opts.Options))
	for i, option := range opts.Options {
		chosen := false
		for _, d := range opts.Defaults {
			if option == d {
				chosen = true
			}
		}

		options[i] = &multiSelectOption{
			option: option,
			chosen: chosen,
		}
	}

	initialModel := multiSelectModel{
		search:  textinput.New(),
		options: options,
		message: opts.Message,
	}

	initialModel.search.Prompt = ""
	initialModel.search.Focus()

	m, err := tea.NewProgram(
		initialModel,
		tea.WithInput(inv.Stdin),
		tea.WithOutput(inv.Stdout),
	).Run()

	values := []string{}
	if m, ok := m.(multiSelectModel); ok {
		if m.canceled {
			return values, Canceled
		}

		values = m.selectedOptions()
	}
	return values, err
}

type multiSelectOption struct {
	option string
	chosen bool
}

type multiSelectModel struct {
	search   textinput.Model
	options  []*multiSelectOption
	cursor   int
	message  string
	canceled bool
	selected bool
}

func (multiSelectModel) Init() tea.Cmd {
	return nil
}

//nolint:revive // For same reason as previous Update definition
func (m multiSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.Type {
		case tea.KeyCtrlC:
			m.canceled = true
			return m, tea.Quit

		case tea.KeyEnter:
			if len(m.options) != 0 {
				m.selected = true
				return m, tea.Quit
			}

		case tea.KeySpace:
			options := m.filteredOptions()
			if len(options) != 0 {
				options[m.cursor].chosen = !options[m.cursor].chosen
			}

		case tea.KeyUp:
			options := m.filteredOptions()
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = len(options) - 1
			}

		case tea.KeyDown:
			options := m.filteredOptions()
			if m.cursor < len(options)-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}

		case tea.KeyRight:
			options := m.filteredOptions()
			for _, option := range options {
				option.chosen = false
			}

		case tea.KeyLeft:
			options := m.filteredOptions()
			for _, option := range options {
				option.chosen = false
			}

		default:
			oldSearch := m.search.Value()
			m.search, cmd = m.search.Update(msg)

			// If the search query has changed then we need to ensure
			// the cursor is still pointing at a valid option.
			if m.search.Value() != oldSearch {
				options := m.filteredOptions()
				if m.cursor > len(options)-1 {
					m.cursor = max(0, len(options)-1)
				}
			}
		}
	}

	return m, cmd
}

func (m multiSelectModel) View() string {
	var s string

	msg := pretty.Sprintf(pretty.Bold(), "? %s", m.message)

	if !m.selected {
		s += fmt.Sprintf("%s %s[Use arrows to move, space to select, <right> to all, <left> to none, type to filter]\n", msg, m.search.View())

		for i, option := range m.filteredOptions() {
			cursor := "  "
			chosen := "[ ]"
			o := option.option

			if m.cursor == i {
				cursor = pretty.Sprint(pretty.FgColor(Green), "> ")
				chosen = pretty.Sprint(pretty.FgColor(Green), "[ ]")
				o = pretty.Sprint(pretty.FgColor(Green), o)
			}

			if option.chosen {
				chosen = pretty.Sprint(pretty.FgColor(Green), "[x]")
			}

			s += fmt.Sprintf(
				"%s%s %s\n",
				cursor,
				chosen,
				o,
			)
		}
	} else {
		selected := pretty.Sprint(DefaultStyles.Keyword, strings.Join(m.selectedOptions(), ", "))

		s += fmt.Sprintf("%s %s\n", msg, selected)
	}

	return s
}

func (m multiSelectModel) filteredOptions() []*multiSelectOption {
	options := []*multiSelectOption{}
	for _, o := range m.options {
		filter := strings.ToLower(m.search.Value())
		option := strings.ToLower(o.option)

		if strings.Contains(option, filter) {
			options = append(options, o)
		}
	}
	return options
}

func (m multiSelectModel) selectedOptions() []string {
	selected := []string{}
	for _, o := range m.options {
		if o.chosen {
			selected = append(selected, o.option)
		}
	}
	return selected
}
