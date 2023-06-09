package cliui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/codersdk"
)

func RichParameter(inv *clibase.Invocation, templateVersionParameter codersdk.TemplateVersionParameter) (string, error) {
	label := templateVersionParameter.Name
	if templateVersionParameter.DisplayName != "" {
		label = templateVersionParameter.DisplayName
	}

	_, _ = fmt.Fprintln(inv.Stdout, DefaultStyles.Bold.Render(label))
	if templateVersionParameter.DescriptionPlaintext != "" {
		_, _ = fmt.Fprintln(inv.Stdout, "  "+strings.TrimSpace(strings.Join(strings.Split(templateVersionParameter.DescriptionPlaintext, "\n"), "\n  "))+"\n")
	}

	var err error
	var value string
	if templateVersionParameter.Type == "list(string)" {
		// Move the cursor up a single line for nicer display!
		_, _ = fmt.Fprint(inv.Stdout, "\033[1A")

		var options []string
		err = json.Unmarshal([]byte(templateVersionParameter.DefaultValue), &options)
		if err != nil {
			return "", err
		}

		values, err := MultiSelect(inv, options)
		if err == nil {
			v, err := json.Marshal(&values)
			if err != nil {
				return "", err
			}

			_, _ = fmt.Fprintln(inv.Stdout)
			_, _ = fmt.Fprintln(inv.Stdout, "  "+DefaultStyles.Prompt.String()+DefaultStyles.Field.Render(strings.Join(values, ", ")))
			value = string(v)
		}
	} else if len(templateVersionParameter.Options) > 0 {
		// Move the cursor up a single line for nicer display!
		_, _ = fmt.Fprint(inv.Stdout, "\033[1A")
		var richParameterOption *codersdk.TemplateVersionParameterOption
		richParameterOption, err = RichSelect(inv, RichSelectOptions{
			Options:    templateVersionParameter.Options,
			Default:    templateVersionParameter.DefaultValue,
			HideSearch: true,
		})
		if err == nil {
			_, _ = fmt.Fprintln(inv.Stdout)
			_, _ = fmt.Fprintln(inv.Stdout, "  "+DefaultStyles.Prompt.String()+DefaultStyles.Field.Render(richParameterOption.Name))
			value = richParameterOption.Value
		}
	} else {
		text := "Enter a value"
		if !templateVersionParameter.Required {
			text += fmt.Sprintf(" (default: %q)", templateVersionParameter.DefaultValue)
		}
		text += ":"

		value, err = Prompt(inv, PromptOptions{
			Text: DefaultStyles.Bold.Render(text),
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
		value = templateVersionParameter.DefaultValue
	}

	return value, nil
}

func validateRichPrompt(value string, p codersdk.TemplateVersionParameter) error {
	return codersdk.ValidateWorkspaceBuildParameter(p, &codersdk.WorkspaceBuildParameter{
		Name:  p.Name,
		Value: value,
	}, nil)
}
