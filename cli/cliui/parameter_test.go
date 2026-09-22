package cliui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestRichParameterSensitiveInput(t *testing.T) {
	t.Parallel()

	const (
		defaultValue = "default-secret"
		inputValue   = "entered-secret"
	)

	ctx := testutil.Context(t, testutil.WaitShort)
	ptty := ptytest.New(t)
	result := make(chan string, 1)
	cmd := &serpent.Command{
		Handler: func(inv *serpent.Invocation) error {
			value, err := cliui.RichParameter(inv, codersdk.TemplateVersionParameter{
				Name:      "token",
				Type:      "string",
				Sensitive: true,
			}, "Token", defaultValue)
			if err == nil {
				result <- value
			}
			return err
		},
	}
	inv := cmd.Invoke().WithContext(ctx)
	ptty.Attach(inv)

	done := make(chan error, 1)
	go func() {
		done <- inv.Run()
	}()

	output := ptty.ExpectNoMatchBefore(ctx, defaultValue, "Enter a value:")
	ptty.WriteLine(inputValue)

	require.NoError(t, <-done)
	require.Equal(t, inputValue, <-result)
	assert.NotContains(t, output, defaultValue)
}

func TestRichParameterSensitiveMultiSelectRedactsSelection(t *testing.T) {
	t.Parallel()

	const sensitiveValue = "sensitive-option-value"

	ctx := testutil.Context(t, testutil.WaitShort)
	ptty := ptytest.New(t)
	result := make(chan string, 1)
	cmd := &serpent.Command{
		Handler: func(inv *serpent.Invocation) error {
			value, err := cliui.RichParameter(inv, codersdk.TemplateVersionParameter{
				Name:         "tokens",
				Type:         "list(string)",
				Sensitive:    true,
				DefaultValue: `[]`,
				Options: []codersdk.TemplateVersionParameterOption{
					{Name: "Visible label", Value: sensitiveValue},
				},
			}, "Tokens", `["`+sensitiveValue+`"]`)
			if err == nil {
				result <- value
			}
			return err
		},
	}
	inv := cmd.Invoke().WithContext(ctx)
	ptty.Attach(inv)

	require.NoError(t, inv.Run())
	require.Equal(t, `["`+sensitiveValue+`"]`, <-result)
	output := ptty.ExpectNoMatchBefore(ctx, sensitiveValue, codersdk.RedactedValue)
	assert.NotContains(t, output, sensitiveValue)
}
