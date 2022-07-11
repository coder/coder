package cliui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/bgentry/speakeasy"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

// PromptOptions supply a set of options to the prompt.
type PromptOptions struct {
	Text      string
	Default   string
	Secret    bool
	IsConfirm bool
	Validate  func(string) error
}

const skipPromptFlag = "yes"

func AllowSkipPrompt(cmd *cobra.Command) {
	cmd.Flags().BoolP(skipPromptFlag, "y", false, "Bypass prompts")
}

const (
	ConfirmYes = "yes"
	ConfirmNo  = "no"
)

// Prompt asks the user for input.
func Prompt(cmd *cobra.Command, opts PromptOptions) (string, error) {
	// If the cmd has a "yes" flag for skipping confirm prompts, honor it.
	// If it's not a "Confirm" prompt, then don't skip. As the default value of
	// "yes" makes no sense.
	if opts.IsConfirm && cmd.Flags().Lookup(skipPromptFlag) != nil {
		if skip, _ := cmd.Flags().GetBool(skipPromptFlag); skip {
			return ConfirmYes, nil
		}
	}

	_, _ = fmt.Fprint(cmd.OutOrStdout(), Styles.FocusedPrompt.String()+opts.Text+" ")
	if opts.IsConfirm {
		if len(opts.Default) == 0 {
			opts.Default = ConfirmYes
		}
		renderedYes := Styles.Placeholder.Render(ConfirmYes)
		renderedNo := Styles.Placeholder.Render(ConfirmNo)
		if opts.Default == ConfirmYes {
			renderedYes = Styles.Bold.Render(ConfirmYes)
		} else {
			renderedNo = Styles.Bold.Render(ConfirmNo)
		}
		_, _ = fmt.Fprint(cmd.OutOrStdout(), Styles.Placeholder.Render("("+renderedYes+Styles.Placeholder.Render("/"+renderedNo+Styles.Placeholder.Render(") "))))
	} else if opts.Default != "" {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), Styles.Placeholder.Render("("+opts.Default+") "))
	}
	interrupt := make(chan os.Signal, 1)

	errCh := make(chan error, 1)
	lineCh := make(chan string)
	go func() {
		var line string
		var err error

		inFile, isInputFile := cmd.InOrStdin().(*os.File)
		if opts.Secret && isInputFile && isatty.IsTerminal(inFile.Fd()) {
			// we don't install a signal handler here because speakeasy has its own
			line, err = speakeasy.Ask("")
		} else {
			signal.Notify(interrupt, os.Interrupt)
			defer signal.Stop(interrupt)

			reader := bufio.NewReader(cmd.InOrStdin())
			line, err = reader.ReadString('\n')

			// Check if the first line beings with JSON object or array chars.
			// This enables multiline JSON to be pasted into an input, and have
			// it parse properly.
			if err == nil && (strings.HasPrefix(line, "{") || strings.HasPrefix(line, "[")) {
				line, err = promptJSON(reader, line)
			}
		}
		if err != nil {
			errCh <- err
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			line = opts.Default
		}
		lineCh <- line
	}()

	select {
	case err := <-errCh:
		return "", err
	case line := <-lineCh:
		if opts.IsConfirm && line != "yes" && line != "y" {
			return line, xerrors.Errorf("got %q: %w", line, Canceled)
		}
		if opts.Validate != nil {
			err := opts.Validate(line)
			if err != nil {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), defaultStyles.Error.Render(err.Error()))
				return Prompt(cmd, opts)
			}
		}
		return line, nil
	case <-cmd.Context().Done():
		return "", cmd.Context().Err()
	case <-interrupt:
		// Print a newline so that any further output starts properly on a new line.
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
		return "", Canceled
	}
}

func promptJSON(reader *bufio.Reader, line string) (string, error) {
	var data bytes.Buffer
	for {
		_, _ = data.WriteString(line)
		var rawMessage json.RawMessage
		err := json.Unmarshal(data.Bytes(), &rawMessage)
		if err != nil {
			if err.Error() != "unexpected end of JSON input" {
				// If a real syntax error occurs in JSON,
				// we want to return that partial line to the user.
				err = nil
				line = data.String()
				break
			}

			// Read line-by-line. We can't use a JSON decoder
			// here because it doesn't work by newline, so
			// reads will block.
			line, err = reader.ReadString('\n')
			if err != nil {
				break
			}
			continue
		}
		// Compacting the JSON makes it easier for parsing and testing.
		rawJSON := data.Bytes()
		data.Reset()
		err = json.Compact(&data, rawJSON)
		if err != nil {
			return line, xerrors.Errorf("compact json: %w", err)
		}
		return data.String(), nil
	}
	return line, nil
}
