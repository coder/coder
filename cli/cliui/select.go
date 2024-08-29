package cliui

import (
	"flag"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
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
	// TODO: Check if this is still true for Bubbletea.
	// The survey library used *always* fails when testing on Windows,
	// as it requires a live TTY (can't be a conpty). We should fork
	// this library to add a dummy fallback, that simply reads/writes
	// to the IO provided. See:
	// https://github.com/AlecAivazis/survey/blob/master/terminal/runereader_windows.go#L94
	if flag.Lookup("test.v") != nil {
		return opts.Options[0], nil
	}

	initialModel := selectModel{
		search:     textinput.New(),
		hideSearch: opts.HideSearch,
		options:    opts.Options,
		height:     opts.Size,
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
			if m.cursor > 0 {
				m.cursor--
			}

		case tea.KeyDown:
			options := m.filteredOptions()
			if m.cursor < len(options)-1 {
				m.cursor++
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

	if m.hideSearch {
		s += "? [Use arrows to move]\n"
	} else {
		s += fmt.Sprintf("? %s [Use arrows to move, type to filter]\n", m.search.View())
	}

	options, start := m.viewableOptions()

	for i, option := range options {
		// Is this the currently selected option?
		cursor := " "
		if m.cursor == start+i {
			cursor = ">"
		}

		s += fmt.Sprintf("%s %s\n", cursor, option)
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
		prefix := strings.ToLower(m.search.Value())
		option := strings.ToLower(o)

		if strings.HasPrefix(option, prefix) {
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
	// Similar hack is applied to Select()
	if flag.Lookup("test.v") != nil {
		return opts.Defaults, nil
	}

	options := make([]multiSelectOption, len(opts.Options))
	for i, option := range opts.Options {
		options[i].option = option
	}

	initialModel := multiSelectModel{
		search:  textinput.New(),
		options: options,
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

		for _, option := range m.options {
			if option.chosen {
				values = append(values, option.option)
			}
		}
	}
	return values, err
}

type multiSelectOption struct {
	option string
	chosen bool
}

type multiSelectModel struct {
	search   textinput.Model
	options  []multiSelectOption
	cursor   int
	canceled bool
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
				return m, tea.Quit
			}

		case tea.KeySpace:
			if len(m.options) != 0 {
				m.options[m.cursor].chosen = true
			}

		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}

		case tea.KeyDown:
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}

		case tea.KeyRight:
			for i := range m.options {
				m.options[i].chosen = true
			}

		case tea.KeyLeft:
			for i := range m.options {
				m.options[i].chosen = false
			}

		default:
			oldSearch := m.search.Value()
			m.search, cmd = m.search.Update(msg)

			// If the search query has changed then we need to ensure
			// the cursor is still pointing at a valid option.
			if m.search.Value() != oldSearch {
				if m.cursor > len(m.options)-1 {
					m.cursor = max(0, len(m.options)-1)
				}
			}
		}
	}

	return m, cmd
}

func (m multiSelectModel) View() string {
	s := fmt.Sprintf("? %s [Use arrows to move, space to select, <right> to all, <left> to none, type to filter]\n", m.search.View())

	for i, option := range m.options {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}

		chosen := "[ ]"
		if option.chosen {
			chosen = "[x]"
		}

		s += fmt.Sprintf("%s %s %s\n", cursor, chosen, option.option)
	}

	return s
}
