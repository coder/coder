package chats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

	steps := 0
	finishReason := "unknown"

	defer func() {
		env := StreamEventEnvelope{
			Type:         EnvelopeTypeStreamEvent,
			RunID:        runID,
			Event:        "run_finished",
			FinishReason: finishReason,
			Steps:        steps,
		}
		content, err := MarshalEnvelope(env)
		if err != nil {
			log.Error(ctx, "marshal run_finished envelope", slog.Error(err))
			return
		}
		_, err = r.opts.DB.InsertChatMessage(ctx, database.InsertChatMessageParams{
			ChatID:    chat.ID,
			CreatedAt: time.Now().UTC(),
			Role:      "system",
			Content:   content,
		})
		if err != nil {
			log.Error(ctx, "persist run_finished", slog.Error(err))
		}
	}()

	// Persist run_started.
	{
		env := StreamEventEnvelope{Type: EnvelopeTypeStreamEvent, RunID: runID, Event: "run_started"}
		content, err := MarshalEnvelope(env)
		if err != nil {
			finishReason = "error"
			_ = r.persistError(ctx, chat.ID, runID, "marshal_error", err.Error(), false)
			return err
		}
		_, err = r.opts.DB.InsertChatMessage(ctx, database.InsertChatMessageParams{
			ChatID:    chat.ID,
			CreatedAt: time.Now().UTC(),
			Role:      "system",
			Content:   content,
		})
		if err != nil {
			finishReason = "error"
			return err
		}
	}

	if r.opts.AccessURL == nil {
		finishReason = "error"
		return xerrors.New("chat runner access url is nil")
	}

	llm, err := r.opts.LLMFactory.New(chat.Provider, r.httpClient())
	if err != nil {
		finishReason = "error"
		_ = r.persistError(ctx, chat.ID, runID, "llm_init_failed", err.Error(), false)
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
		finishReason = "error"
		return xerrors.Errorf("list chat messages: %w", err)
	}
	messages, err := MessagesFromDB(rows)
	if err != nil {
		finishReason = "error"
		return xerrors.Errorf("reconstruct messages from db: %w", err)
	}

	for step := 0; ; step++ {
		steps = step + 1

		req := LLMRequest{
			Model:    chat.Model,
			Messages: messages,
			Tools:    tools,
		}

		stream, err := llm.StreamChat(ctx, req)
		if err != nil {
			finishReason = "error"
			_ = r.persistError(ctx, chat.ID, runID, "upstream_error", err.Error(), true)
			return err
		}

		stream = stream.WithToolCalling(func(call aisdk.ToolCall) any {
			tool, ok := toolByName[call.Name]
			if !ok {
				_ = r.persistToolResult(ctx, chat.ID, runID, steps, call.ID, call.Name, json.RawMessage("null"), fmt.Sprintf("unknown tool: %s", call.Name), 0)
				return map[string]any{"error": fmt.Sprintf("unknown tool: %s", call.Name)}
			}

			argsJSON, err := json.Marshal(call.Args)
			if err != nil {
				_ = r.persistToolResult(ctx, chat.ID, runID, steps, call.ID, call.Name, json.RawMessage("null"), fmt.Sprintf("marshal tool args: %v", err), 0)
				return map[string]any{"error": fmt.Sprintf("marshal tool args: %v", err)}
			}

			_ = r.persistToolCall(ctx, chat.ID, runID, steps, call.ID, call.Name, argsJSON)

			start := time.Now()
			out, err := tool.Handler(ctx, deps, argsJSON)
			dur := time.Since(start).Milliseconds()

			if err != nil {
				_ = r.persistToolResult(ctx, chat.ID, runID, steps, call.ID, call.Name, json.RawMessage("null"), err.Error(), dur)
				return map[string]any{"error": err.Error()}
			}

			_ = r.persistToolResult(ctx, chat.ID, runID, steps, call.ID, call.Name, out, "", dur)

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
				finishReason = "error"
				_ = r.persistError(ctx, chat.ID, runID, "stream_error", err.Error(), true)
				return err
			}
			r.opts.Hub.Publish(chat.ID, StreamEvent{RunID: runID, Part: part})
		}

		for _, msg := range acc.Messages() {
			env := MessageEnvelope{Type: EnvelopeTypeMessage, RunID: runID, Message: msg}
			content, err := MarshalEnvelope(env)
			if err != nil {
				finishReason = "error"
				return err
			}
			_, err = r.opts.DB.InsertChatMessage(ctx, database.InsertChatMessageParams{
				ChatID:    chat.ID,
				CreatedAt: time.Now().UTC(),
				Role:      "assistant",
				Content:   content,
			})
			if err != nil {
				finishReason = "error"
				return err
			}
		}

		usageEnv := StreamEventEnvelope{Type: EnvelopeTypeStreamEvent, RunID: runID, Event: "usage", Usage: acc.Usage(), Step: steps}
		if usageContent, err := MarshalEnvelope(usageEnv); err == nil {
			_, _ = r.opts.DB.InsertChatMessage(ctx, database.InsertChatMessageParams{
				ChatID:    chat.ID,
				CreatedAt: time.Now().UTC(),
				Role:      "system",
				Content:   usageContent,
			})
		}

		messages = append(messages, acc.Messages()...)
		if acc.FinishReason() != aisdk.FinishReasonToolCalls {
			finishReason = string(acc.FinishReason())
			if finishReason == "" {
				finishReason = "stop"
			}
			return nil
		}
	}
}

func (r *Runner) persistError(ctx context.Context, chatID uuid.UUID, runID, code, msg string, retryable bool) error {
	env := StreamEventEnvelope{Type: EnvelopeTypeStreamEvent, RunID: runID, Event: "error", Code: code, Message: msg, Retryable: retryable}
	content, err := MarshalEnvelope(env)
	if err != nil {
		return err
	}
	_, err = r.opts.DB.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:    chatID,
		CreatedAt: time.Now().UTC(),
		Role:      "system",
		Content:   content,
	})
	return err
}

func (r *Runner) persistToolCall(ctx context.Context, chatID uuid.UUID, runID string, step int, toolCallID, toolName string, args json.RawMessage) error {
	env := ToolCallEnvelope{
		Type:       EnvelopeTypeToolCall,
		RunID:      runID,
		Step:       step,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Args:       args,
	}
	content, err := MarshalEnvelope(env)
	if err != nil {
		return err
	}
	_, err = r.opts.DB.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:    chatID,
		CreatedAt: time.Now().UTC(),
		Role:      "tool_call",
		Content:   content,
	})
	return err
}

func (r *Runner) persistToolResult(ctx context.Context, chatID uuid.UUID, runID string, step int, toolCallID, toolName string, result json.RawMessage, toolErr string, durationMS int64) error {
	env := ToolResultEnvelope{
		Type:       EnvelopeTypeToolResult,
		RunID:      runID,
		Step:       step,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Result:     result,
		Error:      toolErr,
		DurationMS: durationMS,
	}
	content, err := MarshalEnvelope(env)
	if err != nil {
		return err
	}
	_, err = r.opts.DB.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:    chatID,
		CreatedAt: time.Now().UTC(),
		Role:      "tool_result",
		Content:   content,
	})
	return err
}

func (r *Runner) httpClient() *http.Client {
	if r.opts.HTTPClient != nil {
		return r.opts.HTTPClient
	}
	return http.DefaultClient
}
