package cliui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"golang.org/x/xerrors"

	"github.com/mattn/go-isatty"

	"github.com/coder/coder/v2/pty"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

// PromptOptions supply a set of options to the prompt.
type PromptOptions struct {
	Text    string
	Default string
	// When true, the input will be masked with asterisks.
	Secret    bool
	IsConfirm bool
	Validate  func(string) error
}

const skipPromptFlag = "yes"

// SkipPromptOption adds a "--yes/-y" flag to the cmd that can be used to skip
// prompts.
func SkipPromptOption() serpent.Option {
	return serpent.Option{
		Flag:          skipPromptFlag,
		FlagShorthand: "y",
		Description:   "Bypass prompts.",
		// Discard
		Value: serpent.BoolOf(new(bool)),
	}
}

const (
	ConfirmYes = "yes"
	ConfirmNo  = "no"
)

// Prompt asks the user for input.
func Prompt(inv *serpent.Invocation, opts PromptOptions) (string, error) {
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
		_, _ = fmt.Fprintf(inv.Stdout, "(%s/%s) ", renderedYes, renderedNo)
	} else if opts.Default != "" {
		_, _ = fmt.Fprintf(inv.Stdout, "(%s) ", pretty.Sprint(DefaultStyles.Placeholder, opts.Default))
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
			line, err = readSecretInput(inFile)
		} else {
			signal.Notify(interrupt, os.Interrupt)
			defer signal.Stop(interrupt)

			line, err = readUntil(inv.Stdin, '\n')

			// Check if the first line beings with JSON object or array chars.
			// This enables multiline JSON to be pasted into an input, and have
			// it parse properly.
			if err == nil && (strings.HasPrefix(line, "{") || strings.HasPrefix(line, "[")) {
				line, err = promptJSON(inv.Stdin, line)
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
			return line, xerrors.Errorf("got %q: %w", line, ErrCanceled)
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
		return "", ErrCanceled
	}
}

func promptJSON(reader io.Reader, line string) (string, error) {
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
			line, err = readUntil(reader, '\n')
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

// readUntil the first occurrence of delim in the input, returning a string containing the data up
// to and including the delimiter. Unlike `bufio`, it only reads until the delimiter and no further
// bytes. If readUntil encounters an error before finding a delimiter, it returns the data read
// before the error and the error itself (often io.EOF). readUntil returns err != nil if and only if
// the returned data does not end in delim.
func readUntil(r io.Reader, delim byte) (string, error) {
	var (
		have []byte
		b    = make([]byte, 1)
	)
	for {
		n, err := r.Read(b)
		if n > 0 {
			have = append(have, b[0])
			if b[0] == delim {
				// match `bufio` in that we only return non-nil if we didn't find the delimiter,
				// regardless of whether we also erred.
				return string(have), nil
			}
		}
		if err != nil {
			return string(have), err
		}
	}
}

// readSecretInput reads secret input from the terminal character by character,
// masking the input with asterisks. It handles special characters like backspace
// and enter appropriately.
func readSecretInput(f *os.File) (string, error) {
	// Set terminal to raw mode
	oldState, err := pty.MakeInputRaw(f.Fd())
	if err != nil {
		return "", err
	}
	defer func() {
		_ = pty.RestoreTerminal(f.Fd(), oldState)
	}()

	// Read input character by character
	buf := make([]byte, 1)
	var line string
	for {
		n, err := f.Read(buf)
		if err != nil || n == 0 {
			return "", ErrCanceled
		}

		// Handle special characters
		switch buf[0] {
		case '\r', '\n': // Enter
			_, _ = f.Write([]byte("\n"))
			return line, nil
		case 3: // Ctrl+C
			_, _ = f.Write([]byte("\n"))
			return "", ErrCanceled
		case 8, 127: // Backspace/Delete
			if len(line) > 0 {
				line = line[:len(line)-1]
				// Move cursor back, print space, move cursor back again
				_, _ = f.Write([]byte("\b \b"))
			}
		default:
			// Only append printable characters
			if buf[0] >= 32 && buf[0] <= 126 {
				line += string(buf[0])
				// Print the mask character
				_, _ = f.Write([]byte("*"))
			}
		}
	}
}
