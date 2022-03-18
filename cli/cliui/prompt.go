package cliui

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/bgentry/speakeasy"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
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

	errCh := make(chan error)
	lineCh := make(chan string)
	go func() {
		var line string
		var err error
		inFile, valid := cmd.InOrStdin().(*os.File)
		if opts.Secret && valid && isatty.IsTerminal(inFile.Fd()) {
			line, err = speakeasy.Ask("")
		} else {
			reader := bufio.NewReader(cmd.InOrStdin())
			line, err = reader.ReadString('\n')
			// Multiline with single quotes!
			if err == nil && strings.HasPrefix(line, "'") {
				rest, err := reader.ReadString('\'')
				if err == nil {
					line += rest
					line = strings.Trim(line, "'")
				}
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
			return line, Canceled
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
		fmt.Fprintln(cmd.OutOrStdout())
		return "", Canceled
	}
}
