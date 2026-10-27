package main //nolint:testpackage // Tests unexported release helpers.

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
)

func TestPrompterNonInteractive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		opts       cliui.PromptOptions
		wantAnswer string
		wantCancel bool
		wantErr    bool
	}{
		{
			name:       "ConfirmWithoutDefaultAccepts",
			opts:       cliui.PromptOptions{Text: "Push tag?", IsConfirm: true},
			wantAnswer: cliui.ConfirmYes,
		},
		{
			name:       "ConfirmDefaultYesAccepts",
			opts:       cliui.PromptOptions{Text: "Create docs update PR?", IsConfirm: true, Default: cliui.ConfirmYes},
			wantAnswer: cliui.ConfirmYes,
		},
		{
			name:       "ConfirmDefaultNoCancels",
			opts:       cliui.PromptOptions{Text: "Continue?", IsConfirm: true, Default: cliui.ConfirmNo},
			wantAnswer: cliui.ConfirmNo,
			wantCancel: true,
		},
		{
			name:       "InputUsesDefault",
			opts:       cliui.PromptOptions{Text: "Version to release", Default: "v2.31.1"},
			wantAnswer: "v2.31.1",
		},
		{
			name:    "InputWithoutDefaultFails",
			opts:    cliui.PromptOptions{Text: "Enter the branch to use"},
			wantErr: true,
		},
		{
			name: "InvalidDefaultFails",
			opts: cliui.PromptOptions{
				Text:     "Version to release",
				Default:  "bogus",
				Validate: func(string) error { return xerrors.New("invalid") },
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stderr bytes.Buffer
			// Stdin is nil so any attempt to read input panics.
			p := &prompter{
				inv:            &serpent.Invocation{Stdout: &stderr, Stderr: &stderr},
				nonInteractive: true,
			}

			answer, err := p.prompt(tt.opts)
			switch {
			case tt.wantCancel:
				require.ErrorIs(t, err, cliui.ErrCanceled)
				require.Equal(t, tt.wantAnswer, answer)
			case tt.wantErr:
				require.Error(t, err)
				require.NotErrorIs(t, err, cliui.ErrCanceled)
			default:
				require.NoError(t, err)
				require.Equal(t, tt.wantAnswer, answer)
			}
		})
	}
}
