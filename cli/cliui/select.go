package cliui

import (
	"errors"
	"flag"
	"io"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
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
func RichSelect(cmd *cobra.Command, richOptions RichSelectOptions) (*codersdk.TemplateVersionParameterOption, error) {
	opts := make([]string, len(richOptions.Options))
	for i, option := range richOptions.Options {
		line := option.Name
		if len(option.Description) > 0 {
			line += ": " + option.Description
		}
		opts[i] = line
	}

	selected, err := Select(cmd, SelectOptions{
		Options:    opts,
		Default:    richOptions.Default,
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
func Select(cmd *cobra.Command, opts SelectOptions) (string, error) {
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
		Reader: cmd.InOrStdin(),
	}, fileReadWriter{
		Writer: cmd.OutOrStdout(),
	}, cmd.OutOrStdout()))
	if errors.Is(err, terminal.InterruptErr) {
		return value, Canceled
	}
	return value, err
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
