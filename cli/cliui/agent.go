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
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

type AgentOptions struct {
	WorkspaceName string
	Fetch         func(context.Context) (codersdk.WorkspaceAgent, error)
	FetchInterval time.Duration
	WarnInterval  time.Duration
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

	if agent.Status == codersdk.WorkspaceAgentConnected {
		return nil
	}

	spin := spinner.New(spinner.CharSets[78], 100*time.Millisecond, spinner.WithColor("fgHiGreen"))
	spin.Writer = writer
	spin.ForceOutput = true
	spin.Suffix = " Waiting for connection from " + Styles.Field.Render(agent.Name) + "..."
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

	var waitMessage string
	messageAfter := time.NewTimer(opts.WarnInterval)
	defer messageAfter.Stop()
	showMessage := func() {
		resourceMutex.Lock()
		defer resourceMutex.Unlock()

		m := waitingMessage(agent)
		if m == waitMessage {
			return
		}
		moveUp := ""
		if waitMessage != "" {
			// If this is an update, move a line up
			// to keep it tidy and aligned.
			moveUp = "\033[1A"
		}
		waitMessage = m

		// Stop the spinner while we write our message.
		spin.Stop()
		// Clear the line and (if necessary) move up a line to write our message.
		_, _ = fmt.Fprintf(writer, "\033[2K%s%s\n\n", moveUp, Styles.Paragraph.Render(Styles.Prompt.String()+waitMessage))
		select {
		case <-ctx.Done():
		default:
			// Safe to resume operation.
			spin.Start()
		}
	}
	go func() {
		select {
		case <-ctx.Done():
		case <-messageAfter.C:
			messageAfter.Stop()
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
			return nil
		case codersdk.WorkspaceAgentTimeout, codersdk.WorkspaceAgentDisconnected:
			showMessage()
		}
	}
}

func waitingMessage(agent codersdk.WorkspaceAgent) string {
	var m string
	switch agent.Status {
	case codersdk.WorkspaceAgentTimeout:
		m = "The workspace agent is having trouble connecting."
	case codersdk.WorkspaceAgentDisconnected:
		m = "The workspace agent lost connection!"
	default:
		// Not a failure state, no troubleshooting necessary.
		return "Don't panic, your workspace is booting up!"
	}
	if agent.TroubleshootingURL != "" {
		return fmt.Sprintf("%s See troubleshooting instructions at: %s", m, agent.TroubleshootingURL)
	}
	return fmt.Sprintf("%s Wait for it to (re)connect or restart your workspace.", m)
}
