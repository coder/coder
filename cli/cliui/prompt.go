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

// Prompt asks the user for input.
func Prompt(cmd *cobra.Command, opts PromptOptions) (string, error) {
	_, _ = fmt.Fprint(cmd.OutOrStdout(), Styles.FocusedPrompt.String()+opts.Text+" ")
	if opts.IsConfirm {
		opts.Default = "yes"
		_, _ = fmt.Fprint(cmd.OutOrStdout(), Styles.Placeholder.Render("("+Styles.Bold.Render("yes")+Styles.Placeholder.Render("/no) ")))
	} else if opts.Default != "" {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), Styles.Placeholder.Render("("+opts.Default+") "))
	}
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)

	errCh := make(chan error, 1)
	lineCh := make(chan string)
	go func() {
		var line string
		var err error

		inFile, isInputFile := cmd.InOrStdin().(*os.File)
		if opts.Secret && isInputFile && isatty.IsTerminal(inFile.Fd()) {
			line, err = speakeasy.Ask("")
		} else {
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
