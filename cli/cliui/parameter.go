package cliui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func RichParameter(inv *serpent.Invocation, templateVersionParameter codersdk.TemplateVersionParameter, defaultOverrides map[string]string) (string, error) {
	label := templateVersionParameter.Name
	if templateVersionParameter.DisplayName != "" {
		label = templateVersionParameter.DisplayName
	}

	if templateVersionParameter.Ephemeral {
		label += pretty.Sprint(DefaultStyles.Warn, " (build option)")
	}

	_, _ = fmt.Fprintln(inv.Stdout, Bold(label))

	if templateVersionParameter.DescriptionPlaintext != "" {
		_, _ = fmt.Fprintln(inv.Stdout, "  "+strings.TrimSpace(strings.Join(strings.Split(templateVersionParameter.DescriptionPlaintext, "\n"), "\n  "))+"\n")
	}

	defaultValue := templateVersionParameter.DefaultValue
	if v, ok := defaultOverrides[templateVersionParameter.Name]; ok {
		defaultValue = v
	}

	var err error
	var value string
	switch {
	case templateVersionParameter.Type == "list(string)":
		// Move the cursor up a single line for nicer display!
		_, _ = fmt.Fprint(inv.Stdout, "\033[1A")

		var options []string
		err = json.Unmarshal([]byte(templateVersionParameter.DefaultValue), &options)
		if err != nil {
			return "", err
		}

		values, err := MultiSelect(inv, MultiSelectOptions{
			Options:  options,
			Defaults: options,
		})
		if err == nil {
			v, err := json.Marshal(&values)
			if err != nil {
				return "", err
			}

			_, _ = fmt.Fprintln(inv.Stdout)
			pretty.Fprintf(
				inv.Stdout,
				DefaultStyles.Prompt, "%s\n", strings.Join(values, ", "),
			)
			value = string(v)
		}
	case len(templateVersionParameter.Options) > 0:
		// Move the cursor up a single line for nicer display!
		_, _ = fmt.Fprint(inv.Stdout, "\033[1A")
		var richParameterOption *codersdk.TemplateVersionParameterOption
		richParameterOption, err = RichSelect(inv, RichSelectOptions{
			Options:    templateVersionParameter.Options,
			Default:    defaultValue,
			HideSearch: true,
		})
		if err == nil {
			_, _ = fmt.Fprintln(inv.Stdout)
			pretty.Fprintf(inv.Stdout, DefaultStyles.Prompt, "%s\n", richParameterOption.Name)
			value = richParameterOption.Value
		}
	default:
		text := "Enter a value"
		if !templateVersionParameter.Required {
			text += fmt.Sprintf(" (default: %q)", defaultValue)
		}
		text += ":"

		value, err = Prompt(inv, PromptOptions{
			Text: Bold(text),
			Validate: func(value string) error {
				return validateRichPrompt(value, templateVersionParameter)
			},
		})
		value = strings.TrimSpace(value)
	}
	if err != nil {
		return "", err
	}

	// If they didn't specify anything, use the default value if set.
	if len(templateVersionParameter.Options) == 0 && value == "" {
		value = defaultValue
	}

	return value, nil
}

func validateRichPrompt(value string, p codersdk.TemplateVersionParameter) error {
	return codersdk.ValidateWorkspaceBuildParameter(p, &codersdk.WorkspaceBuildParameter{
		Name:  p.Name,
		Value: value,
	}, nil)
}
