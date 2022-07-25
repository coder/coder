package cliui

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/codersdk"
)

func ParameterSchema(cmd *cobra.Command, parameterSchema codersdk.ParameterSchema) (string, error) {
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), Styles.Bold.Render("var."+parameterSchema.Name))
	if parameterSchema.Description != "" {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+strings.TrimSpace(strings.Join(strings.Split(parameterSchema.Description, "\n"), "\n  "))+"\n")
	}

	var err error
	var options []string
	if parameterSchema.ValidationCondition != "" {
		options, _, err = parameter.Contains(parameterSchema.ValidationCondition)
		if err != nil {
			return "", err
		}
	}
	var value string
	if len(options) > 0 {
		// Move the cursor up a single line for nicer display!
		_, _ = fmt.Fprint(cmd.OutOrStdout(), "\033[1A")
		value, err = Select(cmd, SelectOptions{
			Options:    options,
			Default:    parameterSchema.DefaultSourceValue,
			HideSearch: true,
		})
		if err == nil {
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+Styles.Prompt.String()+Styles.Field.Render(value))
		}
	} else {
		text := "Enter a value"
		if parameterSchema.DefaultSourceValue != "" {
			text += fmt.Sprintf(" (default: %q)", parameterSchema.DefaultSourceValue)
		}
		text += ":"

		value, err = Prompt(cmd, PromptOptions{
			Text: Styles.Bold.Render(text),
		})
		value = strings.TrimSpace(value)
	}
	if err != nil {
		return "", err
	}

	// If they didn't specify anything, use the default value if set.
	if len(options) == 0 && value == "" {
		value = parameterSchema.DefaultSourceValue
	}

	return value, nil
}
