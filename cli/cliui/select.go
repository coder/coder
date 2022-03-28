package cliui

import (
	"errors"
	"io"
	"strings"
	"text/template"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

type SelectOptions struct {
	Options    []string
	Size       int
	HideSearch bool
}

// Select displays a list of user options.
func Select(cmd *cobra.Command, opts SelectOptions) (string, error) {
	selector := promptui.Select{
		Label: "",
		Items: opts.Options,
		Size:  opts.Size,
		Searcher: func(input string, index int) bool {
			option := opts.Options[index]
			name := strings.Replace(strings.ToLower(option), " ", "", -1)
			input = strings.Replace(strings.ToLower(input), " ", "", -1)

			return strings.Contains(name, input)
		},
		HideHelp: opts.HideSearch,
		Stdin:    io.NopCloser(cmd.InOrStdin()),
		Stdout:   &writeCloser{cmd.OutOrStdout()},
		Templates: &promptui.SelectTemplates{
			FuncMap: template.FuncMap{
				"faint": func(value interface{}) string {
					//nolint:forcetypeassert
					return Styles.Placeholder.Render(value.(string))
				},
				"subtle": func(value interface{}) string {
					//nolint:forcetypeassert
					return defaultStyles.Subtle.Render(value.(string))
				},
				"selected": func(value interface{}) string {
					//nolint:forcetypeassert
					return defaultStyles.Keyword.Render("> " + value.(string))
					// return defaultStyles.SelectedMenuItem.Render("> " + value.(string))
				},
			},
			Active:   "{{ . | selected }}",
			Inactive: "  {{ . }}",
			Label:    "{{.}}",
			Selected: "{{ \"\" }}",
			Help:     `{{ "Use" | faint }} {{ .SearchKey | faint }} {{ "to toggle search" | faint }}`,
		},
		HideSelected: true,
	}

	_, result, err := selector.Run()
	if errors.Is(err, promptui.ErrAbort) || errors.Is(err, promptui.ErrInterrupt) {
		return result, Canceled
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

type writeCloser struct {
	io.Writer
}

func (*writeCloser) Close() error {
	return nil
}
