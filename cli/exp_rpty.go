package cli

import (
	"errors"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
	"golang.org/x/term"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/pty"
	"github.com/coder/serpent"
)
func (r *RootCmd) rptyCommand() *serpent.Command {
	var (
		client = new(codersdk.Client)

		args   handleRPTYArgs
	)
	cmd := &serpent.Command{
		Handler: func(inv *serpent.Invocation) error {
			if r.disableDirect {
				return errors.New("direct connections are disabled, but you can try websocat ;-)")

			}
			args.NamedWorkspace = inv.Args[0]
			args.Command = inv.Args[1:]
			return handleRPTY(inv, client, args)
		},
		Long: "Establish an RPTY session with a workspace/agent. This uses the same mechanism as the Web Terminal.",
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(1, -1),
			r.InitClient(client),
		),
		Options: []serpent.Option{
			{
				Name:          "container",
				Description:   "The container name or ID to connect to.",
				Flag:          "container",
				FlagShorthand: "c",
				Default:       "",
				Value:         serpent.StringOf(&args.Container),
			},
			{
				Name:          "container-user",
				Description:   "The user to connect as.",
				Flag:          "container-user",
				FlagShorthand: "u",
				Default:       "",
				Value:         serpent.StringOf(&args.ContainerUser),
			},
			{
				Name:          "reconnect",
				Description:   "The reconnect ID to use.",
				Flag:          "reconnect",
				FlagShorthand: "r",
				Default:       "",
				Value:         serpent.StringOf(&args.ReconnectID),
			},
		},
		Short: "Establish an RPTY session with a workspace/agent.",
		Use:   "rpty",
	}
	return cmd
}
type handleRPTYArgs struct {
	Command        []string
	Container      string

	ContainerUser  string
	NamedWorkspace string
	ReconnectID    string

}
func handleRPTY(inv *serpent.Invocation, client *codersdk.Client, args handleRPTYArgs) error {
	ctx, cancel := context.WithCancel(inv.Context())
	defer cancel()
	var reconnectID uuid.UUID
	if args.ReconnectID != "" {
		rid, err := uuid.Parse(args.ReconnectID)
		if err != nil {

			return fmt.Errorf("invalid reconnect ID: %w", err)
		}
		reconnectID = rid
	} else {

		reconnectID = uuid.New()
	}
	ws, agt, err := getWorkspaceAndAgent(ctx, inv, client, true, args.NamedWorkspace)
	if err != nil {
		return err
	}
	var ctID string
	if args.Container != "" {
		cts, err := client.WorkspaceAgentListContainers(ctx, agt.ID, nil)
		if err != nil {
			return err
		}
		for _, ct := range cts.Containers {
			if ct.FriendlyName == args.Container || ct.ID == args.Container {
				ctID = ct.ID

				break
			}
		}
		if ctID == "" {
			return fmt.Errorf("container %q not found", args.Container)
		}
	}
	if err := cliui.Agent(ctx, inv.Stderr, agt.ID, cliui.AgentOptions{
		FetchInterval: 0,
		Fetch:         client.WorkspaceAgent,
		Wait:          false,
	}); err != nil {
		return err
	}
	// Get the width and height of the terminal.
	var termWidth, termHeight uint16
	stdoutFile, validOut := inv.Stdout.(*os.File)

	if validOut && isatty.IsTerminal(stdoutFile.Fd()) {
		w, h, err := term.GetSize(int(stdoutFile.Fd()))
		if err == nil {
			//nolint: gosec
			termWidth, termHeight = uint16(w), uint16(h)
		}
	}
	// Set stdin to raw mode so that control characters work.

	stdinFile, validIn := inv.Stdin.(*os.File)
	if validIn && isatty.IsTerminal(stdinFile.Fd()) {
		inState, err := pty.MakeInputRaw(stdinFile.Fd())
		if err != nil {
			return fmt.Errorf("failed to set input terminal to raw mode: %w", err)
		}
		defer func() {
			_ = pty.RestoreTerminal(stdinFile.Fd(), inState)
		}()
	}
	conn, err := workspacesdk.New(client).AgentReconnectingPTY(ctx, workspacesdk.WorkspaceAgentReconnectingPTYOpts{

		AgentID:       agt.ID,
		Reconnect:     reconnectID,
		Command:       strings.Join(args.Command, " "),
		Container:     ctID,
		ContainerUser: args.ContainerUser,
		Width:         termWidth,
		Height:        termHeight,
	})
	if err != nil {
		return fmt.Errorf("open reconnecting PTY: %w", err)
	}
	defer conn.Close()

	cliui.Infof(inv.Stderr, "Connected to %s (agent id: %s)", args.NamedWorkspace, agt.ID)
	cliui.Infof(inv.Stderr, "Reconnect ID: %s", reconnectID)
	closeUsage := client.UpdateWorkspaceUsageWithBodyContext(ctx, ws.ID, codersdk.PostWorkspaceUsageRequest{
		AgentID: agt.ID,
		AppName: codersdk.UsageAppNameReconnectingPty,
	})
	defer closeUsage()
	br := bufio.NewScanner(inv.Stdin)
	// Split on bytes, otherwise you have to send a newline to flush the buffer.
	br.Split(bufio.ScanBytes)
	je := json.NewEncoder(conn)
	go func() {
		for br.Scan() {
			if err := je.Encode(map[string]string{

				"data": br.Text(),
			}); err != nil {
				return
			}
		}
	}()
	windowChange := listenWindowSize(ctx)
	go func() {

		for {
			select {
			case <-ctx.Done():
				return
			case <-windowChange:

			}
			width, height, err := term.GetSize(int(stdoutFile.Fd()))
			if err != nil {
				continue
			}
			if err := je.Encode(map[string]int{
				"width":  width,
				"height": height,
			}); err != nil {
				cliui.Errorf(inv.Stderr, "Failed to send window size: %v", err)

			}
		}
	}()
	_, _ = io.Copy(inv.Stdout, conn)
	cancel()
	_ = conn.Close()
	_, _ = fmt.Fprintf(inv.Stderr, "Connection closed\n")
	return nil
}
