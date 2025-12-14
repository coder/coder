package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdslog "log/slog"
	"net/http"
	"os/exec"
	"sync"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
	"github.com/coder/websocket"
)

func (r *RootCmd) experimentalAcpCommand() *serpent.Command {
	return &serpent.Command{
		Use:   "acp",
		Short: "Experimental commands for ACP (Agent Communication Protocol)",
		Long:  `Experimental commands for ACP (Agent Communication Protocol).`,
		Children: []*serpent.Command{
			r.experimentalAcpStdioWsCommand(),
			r.experimentalAcpClientCommand(),
		},
	}
}

func (r *RootCmd) experimentalAcpStdioWsCommand() *serpent.Command {
	var (
		hostArg string
		portArg int64
	)

	cmd := &serpent.Command{
		Use:   "stdio-ws <command> [args...]",
		Short: "Bridge a stdio JSON-RPC API to WebSocket (experimental POC)",
		Long: `Starts a subprocess and bridges its stdio JSON-RPC protocol to WebSocket, enabling integration with Coder's network-based architecture.

This is a simplified proof of concept that only supports a single client at a time.

Example usage in coder_script:
  export PORT=8080
  export HOST=0.0.0.0
  coder exp acp stdio-ws -- gemini --experimental-acp`,
		Handler: func(inv *serpent.Invocation) error {
			if len(inv.Args) == 0 {
				return xerrors.New("command required: specify child command to run")
			}

			childCmd, childArgs := inv.Args[0], inv.Args[1:]

			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()
			logger := slog.Make(sloghuman.Sink(inv.Stderr)).Named("stdio-ws")
			if r.verbose {
				logger = logger.Leveled(slog.LevelDebug)
			}

			child := exec.CommandContext(ctx, childCmd, childArgs...)
			childStdin, err := child.StdinPipe()
			if err != nil {
				return xerrors.Errorf("getting child stdin: %w", err)
			}
			childStdout, err := child.StdoutPipe()
			if err != nil {
				return xerrors.Errorf("getting child stdout: %w", err)
			}
			childStderr, err := child.StderrPipe()
			if err != nil {
				return xerrors.Errorf("getting child stderr: %w", err)
			}

			// Log child stderr
			stderrReader := bufio.NewScanner(childStderr)
			go func() {
				for stderrReader.Scan() {
					logger.Info(ctx, "child stderr", slog.F("msg", stderrReader.Text()))
				}
				if err := stderrReader.Err(); err != nil {
					logger.Error(ctx, "reading child stderr", slog.Error(err))
				}
			}()
			if err := child.Start(); err != nil {
				return xerrors.Errorf("starting child process: %w", err)
			}
			defer func() {
				if err := child.Process.Kill(); err != nil {
					logger.Error(ctx, "killing child process", slog.Error(err))
				}
			}()

			logger.Info(ctx, "started child process",
				slog.F("pid", child.Process.Pid),
				slog.F("cmd", childCmd),
				slog.F("args", childArgs),
			)

			go func() {
				if err := child.Wait(); err != nil {
					logger.Error(ctx, "child process exited with error", slog.Error(err))
				}
				cancel()
			}()

			wsIn := make(chan []byte)
			wsOut := make(chan []byte)

			// Read from child stdout and send to wsOut
			go func() {
				defer close(wsIn)
				childReader := bufio.NewScanner(childStdout)
				for childReader.Scan() {
					line := childReader.Bytes()
					if !bytes.HasSuffix(line, []byte("\n")) {
						line = append(line, '\n')
					}
					wsOut <- append([]byte(nil), line...)
				}
			}()
			// Read from wsIn and write to child stdin
			go func() {
				defer childStdin.Close()
				for line := range wsIn {
					if !bytes.HasSuffix(line, []byte("\n")) {
						line = append(line, '\n')
					}
					if _, err := childStdin.Write(line); err != nil {
						logger.Error(ctx, "writing to child stdin", slog.Error(err))
						return
					}
				}
			}()

			sb := &stdioBridge{
				log:    logger,
				stdin:  wsIn,
				stdout: wsOut,
			}

			srv := &http.Server{
				Addr:    fmt.Sprintf("%s:%d", hostArg, portArg),
				Handler: sb,
			}

			go func() {
				<-ctx.Done()
				srv.Close()
			}()

			logger.Info(ctx, "starting WebSocket server",
				slog.F("host", hostArg),
				slog.F("port", portArg),
			)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error(ctx, "WebSocket server error", slog.Error(err))
			}
			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "host",
			Env:         "HOST",
			Default:     "0.0.0.0",
			Value:       serpent.StringOf(&hostArg),
			Description: "Host to bind WebSocket server to.",
		},
		{
			Flag:        "port",
			Env:         "PORT",
			Default:     "8080",
			Value:       serpent.Int64Of(&portArg),
			Description: "Port to bind WebSocket server to.",
		},
	}

	return cmd
}

type stdioBridge struct {
	log    slog.Logger
	conn   websocket.Conn
	mu     sync.Mutex // Protects the fields below. NOTE: by design, this limits us to a single client at a time.
	stdin  chan<- []byte
	stdout <-chan []byte
}

func (sb *stdioBridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !sb.mu.TryLock() {
		sb.log.Error(r.Context(), "multiple clients attempted to connect", slog.F("remote_addr", r.RemoteAddr), slog.F("url", r.URL.String()), slog.F("user_agent", r.UserAgent()))
		http.Error(w, "only one client supported at a time", http.StatusServiceUnavailable)
		return
	}
	defer sb.mu.Unlock()

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: buildinfo.IsDev(),
	})
	if err != nil {
		sb.log.Error(r.Context(), "accept WebSocket connection", slog.Error(err))
		return
	}
	defer func() {
		conn.Close(websocket.StatusNormalClosure, "closing connection")
		sb.log.Info(r.Context(), "client disconnected", slog.F("remote_addr", r.RemoteAddr), slog.F("url", r.URL.String()), slog.F("user_agent", r.UserAgent()))
	}()

	sb.log.Info(r.Context(), "client connected", slog.F("remote_addr", r.RemoteAddr), slog.F("url", r.URL.String()), slog.F("user_agent", r.UserAgent()))

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				sb.log.Info(ctx, "stopping ws->child bridge")
				return
			default:
				msgType, data, err := conn.Read(ctx)
				if err != nil {
					sb.log.Error(ctx, "reading from WebSocket", slog.Error(err))
					cancel()
					return
				}
				if msgType != websocket.MessageText && msgType != websocket.MessageBinary {
					sb.log.Error(ctx, "unexpected message type", slog.F("type", msgType))
					continue
				}
				if !json.Valid(data) {
					sb.log.Error(ctx, "invalid JSON message from WebSocket")
					continue
				}
				sb.log.Debug(ctx, "got msg", slog.F("src", "ws"), slog.F("dst", "child"), slog.F("msg", string(data)))
				select {
				case <-ctx.Done():
					return
				case sb.stdin <- data:
				}
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				sb.log.Info(ctx, "stopping child->ws bridge")
				return
			case line := <-sb.stdout:
				sb.log.Debug(ctx, "got msg", slog.F("src", "child"), slog.F("dst", "ws"), slog.F("msg", string(line)))
				if err := conn.Write(ctx, websocket.MessageText, line); err != nil {
					sb.log.Error(ctx, "writing to WebSocket", slog.Error(err))
					cancel()
					return
				}
			}
		}
	}()

	wg.Wait()
}

func (r *RootCmd) experimentalAcpClientCommand() *serpent.Command {
	var (
		url string
	)

	cmd := &serpent.Command{
		Use:   "client <command> [args...]",
		Short: "Run an ACP client over websocket (experimental POC)",
		Long:  `Connects to an ACP server over WebSocket.`,
		Handler: func(inv *serpent.Invocation) error {
			var client acp.Client = &acpClient{inv: inv}

			ctx := inv.Context()
			logger := slog.Make(sloghuman.Sink(inv.Stderr)).Named("acp-client")
			if r.verbose {
				logger = logger.Leveled(slog.LevelDebug)
				stdslog.SetLogLoggerLevel(stdslog.LevelDebug)
			}

			wsConn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{})
			if err != nil {
				return xerrors.Errorf("dialing ACP server: %w", err)
			}
			defer wsConn.Close(websocket.StatusNormalClosure, "closing connection")
			cliui.Infof(inv.Stdout, "Connected to %s", url)
			_, wnc := codersdk.WebsocketNetConn(ctx, wsConn, websocket.MessageText)
			defer wnc.Close()

			csc := acp.NewClientSideConnection(client, wnc, wnc)
			csc.SetLogger(stdslog.Default())
			initResp, err := csc.Initialize(ctx, acp.InitializeRequest{
				ProtocolVersion: acp.ProtocolVersionNumber,
				ClientCapabilities: acp.ClientCapabilities{
					Fs: acp.FileSystemCapability{
						ReadTextFile:  true, // this is a lie
						WriteTextFile: true,
					},
					Terminal: false,
				},
				ClientInfo: &acp.Implementation{
					Name:    "coder-cli",
					Version: buildinfo.Version(),
				},
			})
			if err != nil {
				return xerrors.Errorf("initializing ACP connection: %w", err)
			}
			cliui.Infof(inv.Stdout, "Connected to ACP server (protocol version %d)", initResp.ProtocolVersion)
			sess, err := csc.NewSession(ctx, acp.NewSessionRequest{
				Cwd:        ".",
				McpServers: []acp.McpServer{},
			})
			if err != nil {
				if re, ok := err.(*acp.RequestError); ok {
					return xerrors.Errorf("client error: %s", re.Message)
				}
				return xerrors.Errorf("creating ACP session: %w", err)
			}
			cliui.Infof(inv.Stdout, "ACP session established: %s", sess.SessionId)
			for {
				msg, err := cliui.Prompt(inv, cliui.PromptOptions{})
				if err != nil {
					if errors.Is(err, io.EOF) {
						cliui.Infof(inv.Stdout, "Exiting ACP client.")
						return nil
					}
				}
				_, err = csc.Prompt(ctx, acp.PromptRequest{
					SessionId: sess.SessionId,
					Prompt:    []acp.ContentBlock{acp.TextBlock(msg)},
				})
				if err != nil {
					if re, ok := err.(*acp.RequestError); ok {
						cliui.Errorf(inv.Stderr, "client error: %s", re.Message)
						continue
					} else {
						return xerrors.Errorf("sending prompt: %w", err)
					}
				}
			}
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:        "url",
			Env:         "ACP_SERVER_URL",
			Default:     "ws://localhost:8080",
			Value:       serpent.StringOf(&url),
			Description: "WebSocket URL of the ACP server to connect to.",
		},
	}
	return cmd
}

type acpClient struct {
	inv *serpent.Invocation
}

var _ acp.Client = (*acpClient)(nil)

func (c *acpClient) RequestPermission(ctx context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	opts := make([]string, len(req.Options))
	for i, option := range req.Options {
		opts[i] = option.Name
	}
	resp, err := cliui.Select(c.inv, cliui.SelectOptions{
		Message: fmt.Sprintf("Approve tool call: %s", *req.ToolCall.Title),
		Options: opts,
	})
	if err != nil {
		return acp.RequestPermissionResponse{}, err
	}
	var selectedID acp.PermissionOptionId
	for _, option := range req.Options {
		if option.Name == resp {
			selectedID = option.OptionId
			break
		}
	}
	if selectedID == "" {
		return acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{
				Cancelled: &acp.RequestPermissionOutcomeCancelled{},
			},
		}, nil
	}
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{
			Selected: &acp.RequestPermissionOutcomeSelected{
				OptionId: selectedID,
			},
		},
	}, nil
}

func (c *acpClient) SessionUpdate(ctx context.Context, req acp.SessionNotification) error {
	u := req.Update
	switch {
	case u.AgentMessageChunk != nil:
		content := u.AgentMessageChunk.Content
		if content.Text != nil {
			cliui.Infof(c.inv.Stdout, "[agent_message_chunk] \n%s\n", content.Text.Text)
		}
	case u.ToolCall != nil:
		cliui.Infof(c.inv.Stdout, "\n🔧 %s (%s)\n", u.ToolCall.Title, u.ToolCall.Status)
	case u.ToolCallUpdate != nil:
		cliui.Infof(c.inv.Stdout, "\n🔧 Tool call `%s` updated: %v\n\n", u.ToolCallUpdate.ToolCallId, u.ToolCallUpdate.Status)
	case u.Plan != nil:
		cliui.Infof(c.inv.Stdout, "[plan update]")
	case u.AgentThoughtChunk != nil:
		thought := u.AgentThoughtChunk.Content
		if thought.Text != nil {
			cliui.Infof(c.inv.Stdout, "[agent_thought_chunk] \n%s\n", thought.Text.Text)
		}
	case u.UserMessageChunk != nil:
		cliui.Infof(c.inv.Stdout, "[user_message_chunk]")
	}
	return nil
}

// Below methods not implemented for this POC.
func (c *acpClient) ReadTextFile(ctx context.Context, req acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, fmt.Errorf("not implemented")
}

func (c *acpClient) WriteTextFile(ctx context.Context, req acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, fmt.Errorf("not implemented")
}

func (c *acpClient) CreateTerminal(ctx context.Context, req acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, fmt.Errorf("not implemented")
}

func (c *acpClient) KillTerminalCommand(ctx context.Context, req acp.KillTerminalCommandRequest) (acp.KillTerminalCommandResponse, error) {
	return acp.KillTerminalCommandResponse{}, fmt.Errorf("not implemented")
}

func (c *acpClient) TerminalOutput(ctx context.Context, req acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, fmt.Errorf("not implemented")
}

func (c *acpClient) ReleaseTerminal(ctx context.Context, req acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, fmt.Errorf("not implemented")
}

func (c *acpClient) WaitForTerminalExit(ctx context.Context, req acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, fmt.Errorf("not implemented")
}
