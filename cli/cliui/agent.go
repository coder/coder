package cliui

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

type AgentOptions struct {
	WorkspaceName string
	Fetch         func(context.Context) (codersdk.WorkspaceResource, error)
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
	resource, err := opts.Fetch(ctx)
	if err != nil {
		return xerrors.Errorf("fetch: %w", err)
	}
	if resource.Agent.Status == codersdk.WorkspaceAgentConnected {
		return nil
	}
	if resource.Agent.Status == codersdk.WorkspaceAgentDisconnected {
		opts.WarnInterval = 0
	}
	spin := spinner.New(spinner.CharSets[78], 100*time.Millisecond, spinner.WithColor("fgHiGreen"))
	spin.Writer = writer
	spin.ForceOutput = true
	spin.Suffix = " Waiting for connection from " + Styles.Field.Render(resource.Type+"."+resource.Name) + "..."
	spin.Start()
	defer spin.Stop()

	ticker := time.NewTicker(opts.FetchInterval)
	defer ticker.Stop()
	timer := time.NewTimer(opts.WarnInterval)
	defer timer.Stop()
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		resourceMutex.Lock()
		defer resourceMutex.Unlock()
		message := "Don't panic, your workspace is booting up!"
		if resource.Agent.Status == codersdk.WorkspaceAgentDisconnected {
			message = "The workspace agent lost connection! Wait for it to reconnect or run: " + Styles.Code.Render("coder workspaces rebuild "+opts.WorkspaceName)
		}
		// This saves the cursor position, then defers clearing from the cursor
		// position to the end of the screen.
		_, _ = fmt.Fprintf(writer, "\033[s\r\033[2K%s\n\n", Styles.Paragraph.Render(Styles.Prompt.String()+message))
		defer fmt.Fprintf(writer, "\033[u\033[J")
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		resourceMutex.Lock()
		resource, err = opts.Fetch(ctx)
		if err != nil {
			return xerrors.Errorf("fetch: %w", err)
		}
		if resource.Agent.Status != codersdk.WorkspaceAgentConnected {
			resourceMutex.Unlock()
			continue
		}
		resourceMutex.Unlock()
		return nil
	}
}
