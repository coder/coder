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
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/pretty"
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

// SkipPromptOption adds a "--yes/-y" flag to the cmd that can be used to skip
// prompts.
func SkipPromptOption() clibase.Option {
	return clibase.Option{
		Flag:          skipPromptFlag,
		FlagShorthand: "y",
		Description:   "Bypass prompts.",
		// Discard
		Value: clibase.BoolOf(new(bool)),
	}
}

const (
	ConfirmYes = "yes"
	ConfirmNo  = "no"
)

// Prompt asks the user for input.
func Prompt(inv *clibase.Invocation, opts PromptOptions) (string, error) {
	// If the cmd has a "yes" flag for skipping confirm prompts, honor it.
	// If it's not a "Confirm" prompt, then don't skip. As the default value of
	// "yes" makes no sense.
	if opts.IsConfirm && inv.ParsedFlags().Lookup(skipPromptFlag) != nil {
		if skip, _ := inv.ParsedFlags().GetBool(skipPromptFlag); skip {
			return ConfirmYes, nil
		}
	}

	pretty.Fprintf(inv.Stdout, DefaultStyles.FocusedPrompt, "")
	pretty.Fprintf(inv.Stdout, pretty.Nop, "%s ", opts.Text)
	if opts.IsConfirm {
		if len(opts.Default) == 0 {
			opts.Default = ConfirmYes
		}
		var (
			renderedYes = pretty.Sprint(DefaultStyles.Placeholder, ConfirmYes)
			renderedNo  = pretty.Sprint(DefaultStyles.Placeholder, ConfirmNo)
		)
		if opts.Default == ConfirmYes {
			renderedYes = Bold(ConfirmYes)
		} else {
			renderedNo = Bold(ConfirmNo)
		}
		pretty.Fprintf(inv.Stdout, DefaultStyles.Placeholder, "(%s/%s)", renderedYes, renderedNo)
	} else if opts.Default != "" {
		_, _ = fmt.Fprint(inv.Stdout, pretty.Sprint(DefaultStyles.Placeholder, "("+opts.Default+") "))
	}
	interrupt := make(chan os.Signal, 1)

	if inv.Stdin == nil {
		panic("inv.Stdin is nil")
	}

	errCh := make(chan error, 1)
	lineCh := make(chan string)

	go func() {
		var line string
		var err error

		inFile, isInputFile := inv.Stdin.(*os.File)
		if opts.Secret && isInputFile && isatty.IsTerminal(inFile.Fd()) {
			// we don't install a signal handler here because speakeasy has its own
			line, err = speakeasy.Ask("")
		} else {
			signal.Notify(interrupt, os.Interrupt)
			defer signal.Stop(interrupt)

			reader := bufio.NewReader(inv.Stdin)
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
		select {
		case <-inv.Context().Done():
		case lineCh <- line:
		}
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
				_, _ = fmt.Fprintln(inv.Stdout, pretty.Sprint(DefaultStyles.Error, err.Error()))
				return Prompt(inv, opts)
			}
		}
		return line, nil
	case <-inv.Context().Done():
		return "", inv.Context().Err()
	case <-interrupt:
		// Print a newline so that any further output starts properly on a new line.
		_, _ = fmt.Fprintln(inv.Stdout)
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
