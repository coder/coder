package cliui

import (
	"errors"
	"flag"
	"io"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func init() {
	survey.SelectQuestionTemplate = `
{{- define "option"}}
    {{- "  " }}{{- if eq .SelectedIndex .CurrentIndex }}{{color "green" }}{{ .Config.Icons.SelectFocus.Text }} {{else}}{{color "default"}}  {{end}}
    {{- .CurrentOpt.Value}}
    {{- color "reset"}}
{{end}}

{{- if not .ShowAnswer }}
{{- if .Config.Icons.Help.Text }}
{{- if .FilterMessage }}{{ "Search:" }}{{ .FilterMessage }}
{{- else }}
{{- color "black+h"}}{{- "Type to search" }}{{color "reset"}}
{{- end }}
{{- "\n" }}
{{- end }}
{{- "\n" }}
{{- range $ix, $option := .PageEntries}}
  {{- template "option" $.IterateOption $ix $option}}
{{- end}}
{{- end }}`

	survey.MultiSelectQuestionTemplate = `
{{- define "option"}}
    {{- if eq .SelectedIndex .CurrentIndex }}{{color .Config.Icons.SelectFocus.Format }}{{ .Config.Icons.SelectFocus.Text }}{{color "reset"}}{{else}} {{end}}
    {{- if index .Checked .CurrentOpt.Index }}{{color .Config.Icons.MarkedOption.Format }} {{ .Config.Icons.MarkedOption.Text }} {{else}}{{color .Config.Icons.UnmarkedOption.Format }} {{ .Config.Icons.UnmarkedOption.Text }} {{end}}
    {{- color "reset"}}
    {{- " "}}{{- .CurrentOpt.Value}}
{{end}}
{{- if .ShowHelp }}{{- color .Config.Icons.Help.Format }}{{ .Config.Icons.Help.Text }} {{ .Help }}{{color "reset"}}{{"\n"}}{{end}}
{{- if not .ShowAnswer }}
  {{- "\n"}}
  {{- range $ix, $option := .PageEntries}}
    {{- template "option" $.IterateOption $ix $option}}
  {{- end}}
{{- end}}`
}

type SelectOptions struct {
	Options []string
	// Default will be highlighted first if it's a valid option.
	Default    string
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

	var defaultOption interface{}
	if opts.Default != "" {
		defaultOption = opts.Default
	}

	var value string
	err := survey.AskOne(&survey.Select{
		Options:  opts.Options,
		Default:  defaultOption,
		PageSize: opts.Size,
	}, &value, survey.WithIcons(func(is *survey.IconSet) {
		is.Help.Text = "Type to search"
		if opts.HideSearch {
			is.Help.Text = ""
		}
	}), survey.WithStdio(fileReadWriter{
		Reader: inv.Stdin,
	}, fileReadWriter{
		Writer: inv.Stdout,
	}, inv.Stdout))
	if errors.Is(err, terminal.InterruptErr) {
		return value, Canceled
	}
	return value, err
}

func MultiSelect(inv *serpent.Invocation, items []string) ([]string, error) {
	// Similar hack is applied to Select()
	if flag.Lookup("test.v") != nil {
		return items, nil
	}

	prompt := &survey.MultiSelect{
		Options: items,
		Default: items,
	}

	var values []string
	err := survey.AskOne(prompt, &values, survey.WithStdio(fileReadWriter{
		Reader: inv.Stdin,
	}, fileReadWriter{
		Writer: inv.Stdout,
	}, inv.Stdout))
	if errors.Is(err, terminal.InterruptErr) {
		return nil, Canceled
	}
	return values, err
}

type fileReadWriter struct {
	io.Reader
	io.Writer
}

func (f fileReadWriter) Fd() uintptr {
	if file, ok := f.Reader.(*os.File); ok {
		return file.Fd()
	}
	if file, ok := f.Writer.(*os.File); ok {
		return file.Fd()
	}
	return 0
}
