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

func RichParameter(cmd *cobra.Command, templateVersionParameter codersdk.TemplateVersionParameter) (string, error) {
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), Styles.Bold.Render(templateVersionParameter.Name))
	if templateVersionParameter.DescriptionPlaintext != "" {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+strings.TrimSpace(strings.Join(strings.Split(templateVersionParameter.DescriptionPlaintext, "\n"), "\n  "))+"\n")
	}

	var err error
	var value string
	if len(templateVersionParameter.Options) > 0 {
		// Move the cursor up a single line for nicer display!
		_, _ = fmt.Fprint(cmd.OutOrStdout(), "\033[1A")
		var richParameterOption *codersdk.TemplateVersionParameterOption
		richParameterOption, err = RichSelect(cmd, RichSelectOptions{
			Options:    templateVersionParameter.Options,
			Default:    templateVersionParameter.DefaultValue,
			HideSearch: true,
		})
		if err == nil {
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+Styles.Prompt.String()+Styles.Field.Render(richParameterOption.Name))
			value = richParameterOption.Value
		}
	} else {
		text := "Enter a value"
		if templateVersionParameter.DefaultValue != "" {
			text += fmt.Sprintf(" (default: %q)", templateVersionParameter.DefaultValue)
		}
		text += ":"

		value, err = Prompt(cmd, PromptOptions{
			Text: Styles.Bold.Render(text),
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
	return codersdk.ValidateWorkspaceBuildParameter(p, codersdk.WorkspaceBuildParameter{
		Name:  p.Name,
		Value: value,
	}, nil)
}
