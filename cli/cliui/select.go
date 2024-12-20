package cliui

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

const defaultSelectModelHeight = 7

type terminateMsg struct{}

func installSignalHandler(p *tea.Program) func() {
	ch := make(chan struct{})

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

		defer func() {
			signal.Stop(sig)
			close(ch)
		}()

		for {
			select {
			case <-ch:
				return

			case <-sig:
				p.Send(terminateMsg{})
			}
		}
	}()

	return func() {
		ch <- struct{}{}
	}
}

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
		message:    opts.Message,
	}

	if initialModel.height == 0 {
		initialModel.height = defaultSelectModelHeight
	}

	initialModel.search.Prompt = ""
	initialModel.search.Focus()

	p := tea.NewProgram(
		initialModel,
		tea.WithoutSignalHandler(),
		tea.WithContext(inv.Context()),
		tea.WithInput(inv.Stdin),
		tea.WithOutput(inv.Stdout),
	)

	closeSignalHandler := installSignalHandler(p)
	defer closeSignalHandler()

	m, err := p.Run()
	if err != nil {
		return "", err
	}

	model, ok := m.(selectModel)
	if !ok {
		return "", xerrors.New(fmt.Sprintf("unknown model found %T (%+v)", m, m))
	}

	if model.canceled {
		return "", Canceled
	}

	return model.selected, nil
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
	return nil
}

//nolint:revive // The linter complains about modifying 'm' but this is typical practice for bubbletea
func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case terminateMsg:
		m.canceled = true
		return m, tea.Quit

	case tea.KeyMsg:
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
	var s strings.Builder

	msg := pretty.Sprintf(pretty.Bold(), "? %s", m.message)

	if m.selected != "" {
		selected := pretty.Sprint(DefaultStyles.Keyword, m.selected)
		_, _ = s.WriteString(fmt.Sprintf("%s %s\n", msg, selected))

		return s.String()
	}

	if m.hideSearch {
		_, _ = s.WriteString(fmt.Sprintf("%s [Use arrows to move]\n", msg))
	} else {
		_, _ = s.WriteString(fmt.Sprintf(
			"%s %s[Use arrows to move, type to filter]\n",
			msg,
			m.search.View(),
		))
	}

	options, start := m.viewableOptions()

	for i, option := range options {
		// Is this the currently selected option?
		style := pretty.Wrap("  ", "")
		if m.cursor == start+i {
			style = pretty.Style{
				pretty.Wrap("> ", ""),
				DefaultStyles.Keyword,
			}
		}

		_, _ = s.WriteString(pretty.Sprint(style, option))
		_, _ = s.WriteString("\n")
	}

	return s.String()
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
		bottom = max(0, m.cursor-halfHeight)
		top = min(top, m.cursor+halfHeight+1)
	default:
		bottom = max(0, top-m.height)
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
	Message           string
	Options           []string
	Defaults          []string
	EnableCustomInput bool
}

func MultiSelect(inv *serpent.Invocation, opts MultiSelectOptions) ([]string, error) {
	// Similar hack is applied to Select()
	if flag.Lookup("test.v") != nil {
		return opts.Defaults, nil
	}

	options := make([]*multiSelectOption, len(opts.Options))
	for i, option := range opts.Options {
		chosen := false
		for _, d := range opts.Defaults {
			if option == d {
				chosen = true
				break
			}
		}

		options[i] = &multiSelectOption{
			option: option,
			chosen: chosen,
		}
	}

	initialModel := multiSelectModel{
		search:            textinput.New(),
		options:           options,
		message:           opts.Message,
		enableCustomInput: opts.EnableCustomInput,
	}

	initialModel.search.Prompt = ""
	initialModel.search.Focus()

	p := tea.NewProgram(
		initialModel,
		tea.WithoutSignalHandler(),
		tea.WithContext(inv.Context()),
		tea.WithInput(inv.Stdin),
		tea.WithOutput(inv.Stdout),
	)

	closeSignalHandler := installSignalHandler(p)
	defer closeSignalHandler()

	m, err := p.Run()
	if err != nil {
		return nil, err
	}

	model, ok := m.(multiSelectModel)
	if !ok {
		return nil, xerrors.New(fmt.Sprintf("unknown model found %T (%+v)", m, m))
	}

	if model.canceled {
		return nil, Canceled
	}

	return model.selectedOptions(), nil
}

type multiSelectOption struct {
	option string
	chosen bool
}

type multiSelectModel struct {
	search            textinput.Model
	options           []*multiSelectOption
	cursor            int
	message           string
	canceled          bool
	selected          bool
	isCustomInputMode bool   // track if we're adding a custom option
	customInput       string // store custom input
	enableCustomInput bool   // control whether custom input is allowed
}

func (multiSelectModel) Init() tea.Cmd {
	return nil
}

//nolint:revive // For same reason as previous Update definition
func (m multiSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.isCustomInputMode {
		return m.handleCustomInputMode(msg)
	}

	switch msg := msg.(type) {
	case terminateMsg:
		m.canceled = true
		return m, tea.Quit

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.canceled = true
			return m, tea.Quit

		case tea.KeyEnter:
			// Switch to custom input mode if we're on the "+ Add custom value:" option
			if m.enableCustomInput && m.cursor == len(m.filteredOptions()) {
				m.isCustomInputMode = true
				return m, nil
			}
			if len(m.options) != 0 {
				m.selected = true
				return m, tea.Quit
			}

		case tea.KeySpace:
			options := m.filteredOptions()
			if len(options) != 0 {
				options[m.cursor].chosen = !options[m.cursor].chosen
			}
			// We back out early here otherwise a space will be inserted
			// into the search field.
			return m, nil

		case tea.KeyUp:
			maxIndex := m.getMaxIndex()
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = maxIndex
			}

		case tea.KeyDown:
			maxIndex := m.getMaxIndex()
			if m.cursor < maxIndex {
				m.cursor++
			} else {
				m.cursor = 0
			}

		case tea.KeyRight:
			options := m.filteredOptions()
			for _, option := range options {
				option.chosen = true
			}

		case tea.KeyLeft:
			options := m.filteredOptions()
			for _, option := range options {
				option.chosen = false
			}
		}
	}

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

	return m, cmd
}

func (m multiSelectModel) getMaxIndex() int {
	options := m.filteredOptions()
	if m.enableCustomInput {
		// Include the "+ Add custom value" entry
		return len(options)
	}
	// Includes only the actual options
	return len(options) - 1
}

// handleCustomInputMode manages keyboard interactions when in custom input mode
func (m *multiSelectModel) handleCustomInputMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.Type {
	case tea.KeyEnter:
		return m.handleCustomInputSubmission()

	case tea.KeyCtrlC:
		m.canceled = true
		return m, tea.Quit

	case tea.KeyBackspace:
		return m.handleCustomInputBackspace()

	default:
		m.customInput += keyMsg.String()
		return m, nil
	}
}

// handleCustomInputSubmission processes the submission of custom input
func (m *multiSelectModel) handleCustomInputSubmission() (tea.Model, tea.Cmd) {
	if m.customInput == "" {
		m.isCustomInputMode = false
		return m, nil
	}

	// Clear search to ensure option is visible and cursor points to the new option
	m.search.SetValue("")

	// Check for duplicates
	for i, opt := range m.options {
		if opt.option == m.customInput {
			// If the option exists but isn't chosen, select it
			if !opt.chosen {
				opt.chosen = true
			}

			// Point cursor to the new option
			m.cursor = i

			// Reset custom input mode to disabled
			m.isCustomInputMode = false
			m.customInput = ""
			return m, nil
		}
	}

	// Add new unique option
	m.options = append(m.options, &multiSelectOption{
		option: m.customInput,
		chosen: true,
	})

	// Point cursor to the newly added option
	m.cursor = len(m.options) - 1

	// Reset custom input mode to disabled
	m.customInput = ""
	m.isCustomInputMode = false
	return m, nil
}

// handleCustomInputBackspace handles backspace in custom input mode
func (m *multiSelectModel) handleCustomInputBackspace() (tea.Model, tea.Cmd) {
	if len(m.customInput) > 0 {
		m.customInput = m.customInput[:len(m.customInput)-1]
	}
	return m, nil
}

func (m multiSelectModel) View() string {
	var s strings.Builder

	msg := pretty.Sprintf(pretty.Bold(), "? %s", m.message)

	if m.selected {
		selected := pretty.Sprint(DefaultStyles.Keyword, strings.Join(m.selectedOptions(), ", "))
		_, _ = s.WriteString(fmt.Sprintf("%s %s\n", msg, selected))

		return s.String()
	}

	if m.isCustomInputMode {
		_, _ = s.WriteString(fmt.Sprintf("%s\nEnter custom value: %s\n", msg, m.customInput))
		return s.String()
	}

	_, _ = s.WriteString(fmt.Sprintf(
		"%s %s[Use arrows to move, space to select, <right> to all, <left> to none, type to filter]\n",
		msg,
		m.search.View(),
	))

	options := m.filteredOptions()
	for i, option := range options {
		cursor := "  "
		chosen := "[ ]"
		o := option.option

		if m.cursor == i {
			cursor = pretty.Sprint(DefaultStyles.Keyword, "> ")
			chosen = pretty.Sprint(DefaultStyles.Keyword, "[ ]")
			o = pretty.Sprint(DefaultStyles.Keyword, o)
		}

		if option.chosen {
			chosen = pretty.Sprint(DefaultStyles.Keyword, "[x]")
		}

		_, _ = s.WriteString(fmt.Sprintf(
			"%s%s %s\n",
			cursor,
			chosen,
			o,
		))
	}

	if m.enableCustomInput {
		// Add the "+ Add custom value" option at the bottom
		cursor := "  "
		text := " + Add custom value"
		if m.cursor == len(options) {
			cursor = pretty.Sprint(DefaultStyles.Keyword, "> ")
			text = pretty.Sprint(DefaultStyles.Keyword, text)
		}
		_, _ = s.WriteString(fmt.Sprintf("%s%s\n", cursor, text))
	}
	return s.String()
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
