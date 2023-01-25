package cliui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/muesli/reflow/indent"
	"github.com/muesli/reflow/wordwrap"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

type AgentOptions struct {
	WorkspaceName            string
	Fetch                    func(context.Context) (codersdk.WorkspaceAgent, error)
	FetchInterval            time.Duration
	WarnInterval             time.Duration
	SkipDelayLoginUntilReady bool
}

// Agent displays a spinning indicator that waits for a workspace agent to connect.
func Agent(ctx context.Context, writer io.Writer, opts AgentOptions) error {
	if opts.FetchInterval == 0 {
		opts.FetchInterval = 500 * time.Millisecond
	}
	if opts.WarnInterval == 0 {
		opts.WarnInterval = 30 * time.Second
	}
	var resourceMutex sync.Mutex
	agent, err := opts.Fetch(ctx)
	if err != nil {
		return xerrors.Errorf("fetch: %w", err)
	}

	// Fast path if the agent is ready (avoid showing connecting prompt).
	if agent.Status == codersdk.WorkspaceAgentConnected &&
		(!agent.DelayLoginUntilReady || opts.SkipDelayLoginUntilReady || agent.LifecycleState == codersdk.WorkspaceAgentLifecycleReady) {
		return nil
	}

	spin := spinner.New(spinner.CharSets[78], 100*time.Millisecond, spinner.WithColor("fgHiGreen"))
	spin.Writer = writer
	spin.ForceOutput = true
	spin.Suffix = waitingMessage(agent).Spin
	spin.Start()
	defer spin.Stop()

	ctx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()
	stopSpin := make(chan os.Signal, 1)
	signal.Notify(stopSpin, os.Interrupt)
	defer signal.Stop(stopSpin)
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-stopSpin:
		}
		cancelFunc()
		signal.Stop(stopSpin)
		spin.Stop()
		// nolint:revive
		os.Exit(1)
	}()

	waitMessage := &message{}
	messageAfter := time.NewTimer(opts.WarnInterval)
	defer messageAfter.Stop()
	showMessage := func() {
		resourceMutex.Lock()
		defer resourceMutex.Unlock()

		m := waitingMessage(agent)
		if m.Prompt == waitMessage.Prompt {
			return
		}
		moveUp := ""
		if waitMessage.Prompt != "" {
			// If this is an update, move a line up
			// to keep it tidy and aligned.
			moveUp = "\033[1A"
		}
		waitMessage = m

		// Stop the spinner while we write our message.
		spin.Stop()
		spin.Suffix = waitMessage.Spin
		// Clear the line and (if necessary) move up a line to write our message.
		_, _ = fmt.Fprintf(writer, "\033[2K%s\n%s\n", moveUp, waitMessage.Prompt)
		select {
		case <-ctx.Done():
		default:
			// Safe to resume operation.
			if spin.Suffix != "" {
				spin.Start()
			}
		}
	}
	messageAfterDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			close(messageAfterDone)
		case <-messageAfter.C:
			messageAfter.Stop()
			close(messageAfterDone)
			showMessage()
		}
	}()

	fetchInterval := time.NewTicker(opts.FetchInterval)
	defer fetchInterval.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-fetchInterval.C:
		}
		resourceMutex.Lock()
		agent, err = opts.Fetch(ctx)
		if err != nil {
			resourceMutex.Unlock()
			return xerrors.Errorf("fetch: %w", err)
		}
		resourceMutex.Unlock()
		switch agent.Status {
		case codersdk.WorkspaceAgentConnected:
			// NOTE(mafredri): Once we have access to the workspace agent's
			// startup script logs, we can show them here.
			// https://github.com/coder/coder/issues/2957
			if agent.DelayLoginUntilReady && !opts.SkipDelayLoginUntilReady {
				switch agent.LifecycleState {
				case codersdk.WorkspaceAgentLifecycleCreated, codersdk.WorkspaceAgentLifecycleStarting:
					select {
					case <-messageAfterDone:
						showMessage()
					default:
						// This state is normal, we don't want
						// to show a message prematurely.
					}
				case codersdk.WorkspaceAgentLifecycleReady:
					return nil
				default:
					showMessage()
				}
				continue
			}
			return nil
		case codersdk.WorkspaceAgentTimeout, codersdk.WorkspaceAgentDisconnected:
			showMessage()
		}
	}
}

type message struct {
	Spin         string
	Prompt       string
	Troubleshoot bool
	Error        bool
}

func waitingMessage(agent codersdk.WorkspaceAgent) (m *message) {
	m = &message{
		Prompt: "Don't panic, your workspace is booting up!",
		Spin:   fmt.Sprintf(" Waiting for connection from %s...", Styles.Field.Render(agent.Name)),
	}
	defer func() {
		// We don't want to wrap the troubleshooting URL, so we'll handle word
		// wrapping ourselves (vs using lipgloss).
		w := wordwrap.NewWriter(Styles.Paragraph.GetWidth() - Styles.Paragraph.GetMarginLeft()*2)
		w.Breakpoints = []rune{' ', '\n'}

		_, _ = fmt.Fprint(w, m.Prompt)
		if m.Troubleshoot {
			if agent.TroubleshootingURL != "" {
				_, _ = fmt.Fprintf(w, " See troubleshooting instructions at:\n%s", agent.TroubleshootingURL)
			} else {
				_, _ = fmt.Fprint(w, " Wait for it to (re)connect or restart your workspace.")
			}
		}
		_, _ = fmt.Fprint(w, "\n")

		if m.Error {
			_, _ = fmt.Fprint(w, "\nPress Ctrl+C to exit.\n")
		}

		// We want to prefix the prompt with a caret, but we want text on the
		// following lines to align with the text on the first line (i.e. added
		// spacing).
		ind := " " + Styles.Prompt.String()
		iw := indent.NewWriter(1, func(w io.Writer) {
			_, _ = w.Write([]byte(ind))
			ind = "   " // Set indentation to space after initial prompt.
		})
		_, _ = fmt.Fprint(iw, w.String())
		m.Prompt = iw.String()
	}()

	switch agent.Status {
	case codersdk.WorkspaceAgentTimeout:
		m.Prompt = "The workspace agent is having trouble connecting."
	case codersdk.WorkspaceAgentDisconnected:
		m.Prompt = "The workspace agent lost connection!"
	case codersdk.WorkspaceAgentConnected:
		m.Prompt = "Don't panic, your workspace is starting up!"
		m.Spin = fmt.Sprintf(" Waiting for %s to finish starting up...", Styles.Field.Render(agent.Name))

		switch agent.LifecycleState {
		case codersdk.WorkspaceAgentLifecycleStartTimeout:
			m.Prompt = "The workspace agent is taking longer than expected to start."
		case codersdk.WorkspaceAgentLifecycleStartError:
			m.Spin = ""
			m.Prompt = "The workspace agent ran into a problem during startup."
			m.Error = true
		default:
			// Not a failure state, no troubleshooting necessary.
			return m
		}
	default:
		// Not a failure state, no troubleshooting necessary.
		return m
	}
	m.Troubleshoot = true
	return m
}
