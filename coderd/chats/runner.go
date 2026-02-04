package chats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
)

type RunnerOptions struct {
	DB        database.Store
	Logger    slog.Logger
	AccessURL *url.URL

	HTTPClient *http.Client
	Hub        *Hub

	LLMFactory LLMFactory
	Tools      []toolsdk.GenericTool
}

func NewRunner(opts RunnerOptions) *Runner {
	if opts.Hub == nil {
		opts.Hub = NewHub()
	}
	if opts.LLMFactory == nil {
		opts.LLMFactory = EnvLLMFactory{}
	}
	if opts.Tools == nil {
		opts.Tools = toolsdk.All
	}
	return &Runner{opts: opts}
}

type Runner struct {
	opts RunnerOptions
}

func (r *Runner) Hub() *Hub {
	return r.opts.Hub
}

// Run appends assistant messages to the chat by running an agentic loop.
//
// The stream is best-effort published to the Hub. Durable state is always
// persisted to the database.
func (r *Runner) Run(ctx context.Context, chat database.Chat, coderSessionToken string, runID string) error {
	log := r.opts.Logger.Named("chat_runner").With(
		slog.F("chat_id", chat.ID),
		slog.F("run_id", runID),
		slog.F("provider", chat.Provider),
		slog.F("model", chat.Model),
	)

	if runID == "" {
		return xerrors.New("runID is required")
	}

	if r.opts.AccessURL == nil {
		return xerrors.New("chat runner access url is nil")
	}

	llm, err := r.opts.LLMFactory.New(chat.Provider, r.httpClient())
	if err != nil {
		return err
	}

	coderClient := codersdk.New(r.opts.AccessURL, codersdk.WithSessionToken(coderSessionToken), codersdk.WithHTTPClient(r.httpClient()))
	deps, _ := toolsdk.NewDeps(coderClient)

	toolByName := make(map[string]toolsdk.GenericTool, len(r.opts.Tools))
	tools := make([]aisdk.Tool, 0, len(r.opts.Tools))
	for _, t := range r.opts.Tools {
		toolByName[t.Name] = t
		tools = append(tools, t.Tool)
	}

	rows, err := r.opts.DB.ListChatMessages(ctx, chat.ID)
	if err != nil {
		return xerrors.Errorf("list chat messages: %w", err)
	}
	messages, err := MessagesFromDB(rows)
	if err != nil {
		return xerrors.Errorf("reconstruct messages from db: %w", err)
	}

	for {
		req := LLMRequest{
			Model:    chat.Model,
			Messages: messages,
			Tools:    tools,
		}

		stream, err := llm.StreamChat(ctx, req)
		if err != nil {
			return err
		}

		stream = stream.WithToolCalling(func(call aisdk.ToolCall) any {
			tool, ok := toolByName[call.Name]
			if !ok {
				return map[string]any{"error": fmt.Sprintf("unknown tool: %s", call.Name)}
			}

			if toolRequiresWorkspace(call.Name) {
				workspaceID, err := workspaceIDForToolCall(ctx, coderClient, chat, call)
				if err != nil {
					return map[string]any{"error": err.Error()}
				}
				if workspaceID != uuid.Nil {
					log.Debug(ctx, "waiting for workspace before tool call",
						slog.F("tool", call.Name),
						slog.F("workspace_id", workspaceID),
					)
					if err := waitForWorkspaceReady(ctx, coderClient, workspaceID); err != nil {
						return map[string]any{"error": err.Error()}
					}
				}
			}

			argsJSON, err := json.Marshal(call.Args)
			if err != nil {
				return map[string]any{"error": fmt.Sprintf("marshal tool args: %v", err)}
			}

			out, err := tool.Handler(ctx, deps, argsJSON)
			if err != nil {
				return map[string]any{"error": err.Error()}
			}

			var v any
			if err := json.Unmarshal(out, &v); err != nil {
				return map[string]any{"error": fmt.Sprintf("unmarshal tool result: %v", err)}
			}
			return v
		})

		var acc aisdk.DataStreamAccumulator
		stream = stream.WithAccumulator(&acc)

		for part, err := range stream {
			if err != nil {
				return err
			}
			r.opts.Hub.Publish(chat.ID, StreamEvent{RunID: runID, Part: part})
		}

		for _, msg := range acc.Messages() {
			env := MessageEnvelope{Type: EnvelopeTypeMessage, RunID: runID, Message: msg}
			content, err := MarshalEnvelope(env)
			if err != nil {
				return err
			}
			row, err := r.opts.DB.InsertChatMessage(ctx, database.InsertChatMessageParams{
				ChatID:    chat.ID,
				CreatedAt: time.Now().UTC(),
				Role:      "assistant",
				Content:   content,
			})
			if err != nil {
				return err
			}
			r.opts.Hub.PublishMessage(chat.ID, row)
		}

		messages = append(messages, acc.Messages()...)
		if acc.FinishReason() != aisdk.FinishReasonToolCalls {
			return nil
		}
	}
}

func toolRequiresWorkspace(name string) bool {
	switch name {
	case toolsdk.ToolNameWorkspaceBash,
		toolsdk.ToolNameWorkspaceLS,
		toolsdk.ToolNameWorkspaceReadFile,
		toolsdk.ToolNameWorkspaceWriteFile,
		toolsdk.ToolNameWorkspaceEditFile,
		toolsdk.ToolNameWorkspaceEditFiles,
		toolsdk.ToolNameWorkspacePortForward,
		toolsdk.ToolNameWorkspaceListApps:
		return true
	default:
		return false
	}
}

func workspaceIDForToolCall(
	ctx context.Context,
	client *codersdk.Client,
	chat database.Chat,
	call aisdk.ToolCall,
) (uuid.UUID, error) {
	if workspaceName, ok := call.Args["workspace"].(string); ok && workspaceName != "" {
		return resolveWorkspaceID(ctx, client, workspaceName)
	}
	if workspaceID, ok := call.Args["workspace_id"].(string); ok && workspaceID != "" {
		return resolveWorkspaceID(ctx, client, workspaceID)
	}
	if chat.WorkspaceID.Valid {
		return chat.WorkspaceID.UUID, nil
	}
	return uuid.Nil, nil
}

func resolveWorkspaceID(ctx context.Context, client *codersdk.Client, identifier string) (uuid.UUID, error) {
	if identifier == "" {
		return uuid.Nil, nil
	}
	if workspaceID, err := uuid.Parse(identifier); err == nil {
		return workspaceID, nil
	}

	normalized := toolsdk.NormalizeWorkspaceInput(identifier)
	owner, workspaceName := splitOwnerAndName(normalized)
	workspaceName = strings.SplitN(workspaceName, ".", 2)[0]

	workspace, err := client.WorkspaceByOwnerAndName(ctx, owner, workspaceName, codersdk.WorkspaceOptions{})
	if err != nil {
		return uuid.Nil, err
	}
	return workspace.ID, nil
}

func splitOwnerAndName(identifier string) (owner string, name string) {
	parts := strings.SplitN(identifier, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "me", identifier
}

func waitForWorkspaceReady(ctx context.Context, client *codersdk.Client, workspaceID uuid.UUID) error {
	if workspaceID == uuid.Nil {
		return nil
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		workspace, err := client.Workspace(ctx, workspaceID)
		if err != nil {
			return err
		}

		switch workspace.LatestBuild.Status {
		case codersdk.WorkspaceStatusRunning:
			return nil
		case codersdk.WorkspaceStatusStopped:
			return nil
		case codersdk.WorkspaceStatusFailed,
			codersdk.WorkspaceStatusCanceled,
			codersdk.WorkspaceStatusDeleted:
			return xerrors.Errorf("workspace %q is %s", workspace.Name, workspace.LatestBuild.Status)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) httpClient() *http.Client {
	if r.opts.HTTPClient != nil {
		return r.opts.HTTPClient
	}
	return http.DefaultClient
}
