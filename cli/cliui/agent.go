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

var (
	AgentStartError   = xerrors.New("agent startup exited with non-zero exit status")
	AgentShuttingDown = xerrors.New("agent is shutting down")
)

type AgentOptions struct {
	WorkspaceName string
	Fetch         func(context.Context) (codersdk.WorkspaceAgent, error)
	FetchInterval time.Duration
	WarnInterval  time.Duration
	NoWait        bool // If true, don't wait for the agent to be ready.
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
	// We don't take the fast path for opts.NoWait yet because we want to
	// show the message.
	if agent.Status == codersdk.WorkspaceAgentConnected &&
		(agent.LoginBeforeReady || agent.LifecycleState == codersdk.WorkspaceAgentLifecycleReady) {
		return nil
	}

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	spin := spinner.New(spinner.CharSets[78], 100*time.Millisecond, spinner.WithColor("fgHiGreen"))
	spin.Writer = writer
	spin.ForceOutput = true
	spin.Suffix = waitingMessage(agent, opts).Spin

	waitMessage := &message{}
	showMessage := func() {
		resourceMutex.Lock()
		defer resourceMutex.Unlock()

		m := waitingMessage(agent, opts)
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

	// Fast path for showing the error message even when using no wait,
	// we do this just before starting the spinner to avoid needless
	// spinning.
	if agent.Status == codersdk.WorkspaceAgentConnected &&
		!agent.LoginBeforeReady && opts.NoWait {
		showMessage()
		return nil
	}

	// Start spinning after fast paths are handled.
	if spin.Suffix != "" {
		spin.Start()
	}
	defer spin.Stop()

	warnAfter := time.NewTimer(opts.WarnInterval)
	defer warnAfter.Stop()
	warningShown := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			close(warningShown)
		case <-warnAfter.C:
			close(warningShown)
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
			if !agent.LoginBeforeReady && !opts.NoWait {
				switch agent.LifecycleState {
				case codersdk.WorkspaceAgentLifecycleReady:
					return nil
				case codersdk.WorkspaceAgentLifecycleStartTimeout:
					showMessage()
				case codersdk.WorkspaceAgentLifecycleStartError:
					showMessage()
					return AgentStartError
				case codersdk.WorkspaceAgentLifecycleShuttingDown, codersdk.WorkspaceAgentLifecycleShutdownTimeout,
					codersdk.WorkspaceAgentLifecycleShutdownError, codersdk.WorkspaceAgentLifecycleOff:
					showMessage()
					return AgentShuttingDown
				default:
					select {
					case <-warningShown:
						showMessage()
					default:
						// This state is normal, we don't want
						// to show a message prematurely.
					}
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
}

func waitingMessage(agent codersdk.WorkspaceAgent, opts AgentOptions) (m *message) {
	m = &message{
		Spin:   fmt.Sprintf("Waiting for connection from %s...", Styles.Field.Render(agent.Name)),
		Prompt: "Don't panic, your workspace is booting up!",
	}
	defer func() {
		if agent.Status == codersdk.WorkspaceAgentConnected && opts.NoWait {
			m.Spin = ""
		}
		if m.Spin != "" {
			m.Spin = " " + m.Spin
		}

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
		m.Spin = fmt.Sprintf("Waiting for %s to become ready...", Styles.Field.Render(agent.Name))
		m.Prompt = "Don't panic, your workspace agent has connected and the workspace is getting ready!"
		if opts.NoWait {
			m.Prompt = "Your workspace is still getting ready, it may be in an incomplete state."
		}

		switch agent.LifecycleState {
		case codersdk.WorkspaceAgentLifecycleStartTimeout:
			m.Prompt = "The workspace is taking longer than expected to get ready, the agent startup script is still executing."
		case codersdk.WorkspaceAgentLifecycleStartError:
			m.Spin = ""
			m.Prompt = "The workspace ran into a problem while getting ready, the agent startup script exited with non-zero status."
		default:
			switch agent.LifecycleState {
			case codersdk.WorkspaceAgentLifecycleShutdownTimeout:
				m.Spin = ""
				m.Prompt = "The workspace is shutting down, but is taking longer than expected to shut down and the agent shutdown script is still executing."
				m.Troubleshoot = true
			case codersdk.WorkspaceAgentLifecycleShutdownError:
				m.Spin = ""
				m.Prompt = "The workspace ran into a problem while shutting down, the agent shutdown script exited with non-zero status."
				m.Troubleshoot = true
			case codersdk.WorkspaceAgentLifecycleShuttingDown:
				m.Spin = ""
				m.Prompt = "The workspace is shutting down."
			case codersdk.WorkspaceAgentLifecycleOff:
				m.Spin = ""
				m.Prompt = "The workspace is not running."
			}
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
