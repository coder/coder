package aibridged

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	ant_ssestream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/google/uuid"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared/constant"
	"github.com/tidwall/gjson"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/codersdk"
)

type Bridge struct {
	cfg codersdk.AIBridgeConfig

	httpSrv  *http.Server
	addr     string
	clientFn func() (proto.DRPCAIBridgeDaemonClient, bool)
	logger   slog.Logger
}

func NewBridge(cfg codersdk.AIBridgeConfig, addr string, logger slog.Logger, clientFn func() (proto.DRPCAIBridgeDaemonClient, bool)) *Bridge {
	var bridge Bridge

	mux := &http.ServeMux{}
	mux.HandleFunc("/v1/chat/completions", bridge.proxyOpenAIRequest)
	mux.HandleFunc("/v1/messages", bridge.proxyAnthropicRequest)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
		// TODO: other settings.
	}

	bridge.cfg = cfg
	bridge.httpSrv = srv
	bridge.clientFn = clientFn
	bridge.logger = logger

	return &bridge
}

// ChatCompletionNewParamsWrapper exists because the "stream" param is not included in openai.ChatCompletionNewParams.
type ChatCompletionNewParamsWrapper struct {
	openai.ChatCompletionNewParams `json:""`
	Stream                         bool `json:"stream,omitempty"`
}

func (b ChatCompletionNewParamsWrapper) MarshalJSON() ([]byte, error) {
	type shadow ChatCompletionNewParamsWrapper
	return param.MarshalWithExtras(b, (*shadow)(&b), map[string]any{
		"stream": b.Stream,
	})
}

func (b *ChatCompletionNewParamsWrapper) UnmarshalJSON(raw []byte) error {
	err := b.ChatCompletionNewParams.UnmarshalJSON(raw)
	if err != nil {
		return err
	}

	in := gjson.ParseBytes(raw)
	if stream := in.Get("stream"); stream.Exists() {
		b.Stream = stream.Bool()
		if b.Stream {
			b.ChatCompletionNewParams.StreamOptions = openai.ChatCompletionStreamOptionsParam{
				IncludeUsage: openai.Bool(true), // Always include usage when streaming.
			}
		} else {
			b.ChatCompletionNewParams.StreamOptions = openai.ChatCompletionStreamOptionsParam{}
		}
	} else {
		b.ChatCompletionNewParams.StreamOptions = openai.ChatCompletionStreamOptionsParam{}
	}

	return nil
}

func (b *Bridge) openAITarget() *url.URL {
	u := b.cfg.OpenAIBaseURL.String()
	target, err := url.Parse(u)
	if err != nil {
		panic(fmt.Sprintf("failed to parse %q", u))

	}
	return target
}

func (b *Bridge) proxyOpenAIRequest(w http.ResponseWriter, r *http.Request) {
	sessionID := uuid.New()
	_, _ = fmt.Fprintf(os.Stderr, "[%s] new chat session started\n\n", sessionID)
	defer func() {
		_, _ = fmt.Fprintf(os.Stderr, "[%s] chat session ended\n\n", sessionID)
	}()

	// Required characteristics:
	// 1.  Client-side cancel
	// 2.  No timeout (SSE)
	// 3a. client->coderd conn established
	// 3b. coderd->AI provider conn established
	// 4.  responses from AI provider->coderd must be parsed, optionally reflected back to client
	// 5.  tool calls must be injected and intercepted, transparently to the client
	// 6.  multiple calls can be made to AI provider while holding client->coderd conn open
	// 7.  client->coderd conn must ONLY be closed on client-side disconn or coderd->AI provider non-recoverable error.

	// Allow us to interrupt watch via cancel.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(ctx) // Rewire context for SSE cancellation.

	coderdClient, ok := b.clientFn()
	if !ok {
		// TODO: log issue.
		http.Error(w, "could not acquire coderd client", http.StatusInternalServerError)
		return
	}

	// Parse incoming request, inject tool calls.
	var in ChatCompletionNewParamsWrapper

	body, err := io.ReadAll(r.Body)
	if err != nil {
		b.logger.Error(r.Context(), "failed to read body", slog.Error(err))
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	if err = json.Unmarshal(body, &in); err != nil {
		b.logger.Error(r.Context(), "failed to unmarshal request", slog.Error(err))
		http.Error(w, "failed to unmarshal request", http.StatusInternalServerError)
		return
	}

	if len(in.Messages) > 0 {
		// Find last user message.
		var msg *openai.ChatCompletionUserMessageParam
		for i := len(in.Messages) - 1; i >= 0; i-- {
			m := in.Messages[i]
			if m.OfUser != nil {
				msg = m.OfUser
				break
			}
		}

		if msg != nil {
			message := msg.Content.OfString.String()
			if isCursor, _ := regexp.MatchString("<user_query>", message); isCursor {
				message = b.extractCursorUserQuery(message)
			}

			if _, err = coderdClient.TrackUserPrompts(ctx, &proto.TrackUserPromptsRequest{
				Prompt: message,
			}); err != nil {
				b.logger.Error(r.Context(), "failed to track user prompt", slog.Error(err))
			}
		}
	}

	// Prepend assistant message.
	in.Messages = append([]openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("You are a helpful assistant that explicitly mentions when tool calls are about to be made."),
	}, in.Messages...)

	in.Tools = []openai.ChatCompletionToolParam{
		{
			Function: openai.FunctionDefinitionParam{
				Name:        "get_weather",
				Description: openai.String("Get weather at the given location"),
				Parameters: openai.FunctionParameters{
					"type": "object",
					"properties": map[string]interface{}{
						"location": map[string]string{
							"type": "string",
						},
					},
					"required": []string{"location"},
				},
			},
		},
	}

	client := openai.NewClient()

	if in.Stream {
		streamCtx, streamCancel := context.WithCancelCause(ctx)
		defer streamCancel(xerrors.New("deferred"))

		eventStream := newOpenAIEventStream()

		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			defer func() {
				if err := eventStream.Close(streamCtx); err != nil {
					b.logger.Error(ctx, "error closing stream", slog.Error(err), slog.F("sessionID", sessionID))
				}
			}()

			BasicSSESender(streamCtx, sessionID, eventStream, b.logger.Named("sse-sender")).ServeHTTP(w, r)
		}()

		session := NewOpenAISession()

		stream := client.Chat.Completions.NewStreaming(ctx, in.ChatCompletionNewParams)

		ignoreSubsequent := make(map[string]any)

		var acc openai.ChatCompletionAccumulator
		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			var foundToolCallDelta bool
			for _, c := range chunk.Choices {
				for range c.Delta.ToolCalls {
					foundToolCallDelta = true
					ignoreSubsequent[acc.ID] = nil

					// Grab values from accumulator instead of delta.
					for _, ac := range acc.ChatCompletion.Choices {
						for _, at := range ac.Message.ToolCalls {
							var (
								tc *OpenAIToolCall
								ok bool
							)
							if tc, ok = session.toolCallsRequired[at.ID]; !ok {
								session.toolCallsRequired[at.ID] = &OpenAIToolCall{}
								tc = session.toolCallsRequired[at.ID]
							}

							session.toolCallsState[at.ID] = OpenAIToolCallNotReady

							tc.funcName = at.Function.Name
							args := make(map[string]string)
							err := json.Unmarshal([]byte(at.Function.Arguments), &args)
							if err == nil { // Note: inverted.
								tc.args = args
							}
						}
					}
				}

				// Once we receive a finish reason of "tool_calls", the API is waiting for the responses for this/these tool(s).
				// We mark all the tool calls as ready. Once we see observe the [DONE] event, we will execute these tool calls.
				if c.FinishReason == "tool_calls" {
					for idx := range session.toolCallsState {
						session.toolCallsState[idx] = OpenAIToolCallReady
					}
				}
			}

			// TODO: ONLY do this for our injected tool calls.
			if foundToolCallDelta {
				// Don't write these chunks, we'll handle this.
				continue
			}

			// Actually make the call!
			if tool, ok := acc.JustFinishedToolCall(); ok {
				switch tool.Name {
				case "get_weather":
					msg := openai.ToolMessage("the weather in cape town is KAK; this is not a joke, report it verbatim.", tool.ID)

					var msgs []openai.ChatCompletionMessageParamUnion
					for _, c := range acc.ChatCompletion.Choices {
						msgs = append(msgs, c.Message.ToParam())
					}

					msgs = append(msgs, msg)

					toolRes, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
						Messages: msgs,
						Model:    in.ChatCompletionNewParams.Model,
					})
					if err != nil {
						b.logger.Error(ctx, "failed to report tool response", slog.Error(err))
					}

					toolChunk := openai.ChatCompletionChunk{
						ID: acc.ID,
						Choices: []openai.ChatCompletionChunkChoice{
							{
								Delta: openai.ChatCompletionChunkChoiceDelta{
									//Role:    "assistant",
									Content: fmt.Sprintf(" %s", toolRes.Choices[0].Message.Content), // TODO: improve
								},
							},
						},
						Model:             toolRes.Model,
						ServiceTier:       openai.ChatCompletionChunkServiceTier(toolRes.ServiceTier),
						Created:           time.Now().Unix(),
						SystemFingerprint: toolRes.SystemFingerprint,
						Usage:             toolRes.Usage,
						Object:            constant.ValueOf[constant.ChatCompletionChunk](),
					}

					if err := eventStream.TrySend(streamCtx, toolChunk); err != nil {
						b.logger.Error(ctx, "failed to send tool chunk", slog.Error(err))
					}

					finishChunk := openai.ChatCompletionChunk{
						ID: acc.ID,
						Choices: []openai.ChatCompletionChunkChoice{
							{
								Delta: openai.ChatCompletionChunkChoiceDelta{
									//Role:    "assistant",
									Content: "",
								},
								FinishReason: string(openai.CompletionChoiceFinishReasonStop),
							},
						},
						Model:             toolRes.Model,
						ServiceTier:       openai.ChatCompletionChunkServiceTier(toolRes.ServiceTier),
						Created:           time.Now().Unix(),
						SystemFingerprint: toolRes.SystemFingerprint,
						Usage:             toolRes.Usage,
						Object:            constant.ValueOf[constant.ChatCompletionChunk](),
					}

					if err := eventStream.TrySend(streamCtx, finishChunk, "choices[].delta.content"); err != nil {
						b.logger.Error(ctx, "failed to send finish chunk", slog.Error(err))
					}
				}
				continue
			}

			// TODO: clean this up. Once we receive a tool invocation we need to hijack the conversation, since the client
			// 		 won't be handling the tool call if auto-injected. That means that any subsequent events which wrap
			//		 up the stream need to be ignored because we send those after the tool call is executed and the result
			//		 is appended as if it came from the assistant.
			if _, ok := ignoreSubsequent[acc.ID]; !ok {
				if err := eventStream.TrySend(streamCtx, chunk); err != nil {
					b.logger.Error(ctx, "failed to send reflected chunk", slog.Error(err))
				}
			}
		}

		if err := eventStream.Close(streamCtx); err != nil {
			b.logger.Error(ctx, "failed to close event stream", slog.Error(err))
		}

		if err := stream.Err(); err != nil {
			// TODO: handle error.
			b.logger.Error(ctx, "server stream error", slog.Error(err))
		}

		wg.Wait()

		// Ensure we flush all the remaining data before ending.
		flush(w)

		streamCancel(xerrors.New("gracefully done"))

		select {
		case <-streamCtx.Done():
		}
	} else {
		completion, err := client.Chat.Completions.New(ctx, in.ChatCompletionNewParams)
		if err != nil {
			b.logger.Error(ctx, "chat completion failed", slog.Error(err))
			http.Error(w, "chat completion failed", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK) // TODO: always?
		_, _ = w.Write([]byte(completion.RawJSON()))
	}
}

func (b *Bridge) extractCursorUserQuery(message string) string {
	pat := regexp.MustCompile(`<user_query>(?P<content>[\s\S]*?)</user_query>`)
	match := pat.FindStringSubmatch(message)
	if match != nil {
		// Get the named group by index
		contentIndex := pat.SubexpIndex("content")
		if contentIndex != -1 {
			message = match[contentIndex]
		}
	}
	return strings.TrimSpace(message)
}

func (b *Bridge) proxyAnthropicRequest(w http.ResponseWriter, r *http.Request) {
	coderdClient, ok := b.clientFn()
	if !ok {
		// TODO: log issue.
		http.Error(w, "could not acquire coderd client", http.StatusInternalServerError)
		return
	}

	target, err := url.Parse("https://api.anthropic.com")
	if err != nil {
		http.Error(w, "failed to parse Anthropic URL", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// Add Anthropic-specific headers
		if strings.TrimSpace(req.Header.Get("x-api-key")) == "" {
			req.Header.Set("x-api-key", os.Getenv("ANTHROPIC_API_KEY"))
		}
		req.Header.Set("anthropic-version", "2023-06-01")

		if req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}

		req.Host = target.Host
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host

		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, "could not ready request body", http.StatusBadRequest)
			return
		}
		_ = req.Body.Close()

		var msg anthropic.MessageNewParams
		err = json.NewDecoder(bytes.NewReader(body)).Decode(&msg)
		if err != nil {
			http.Error(w, "could not unmarshal request body", http.StatusBadRequest)
			return
		}

		// TODO: robustness
		if len(msg.Messages) > 0 {
			latest := msg.Messages[len(msg.Messages)-1]
			if len(latest.Content) > 0 {
				if latest.Content[0].OfText != nil {
					_, _ = coderdClient.TrackUserPrompts(r.Context(), &proto.TrackUserPromptsRequest{
						Prompt: latest.Content[0].OfText.Text,
					})
				} else {
					fmt.Println()
				}
			}
		}

		req.Body = io.NopCloser(bytes.NewReader(body))

		fmt.Printf("Proxying %s request to: %s\n", req.Method, req.URL.String())
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return xerrors.Errorf("read response body: %w", err)
		}
		if err = response.Body.Close(); err != nil {
			return xerrors.Errorf("close body: %w", err)
		}

		if !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
			var msg anthropic.Message

			// TODO: check content-encoding to handle others.
			gr, err := gzip.NewReader(bytes.NewReader(body))
			if err != nil {
				return xerrors.Errorf("parse gzip-encoded body: %w", err)
			}

			err = json.NewDecoder(gr).Decode(&msg)
			if err != nil {
				return xerrors.Errorf("parse non-streaming body: %w", err)
			}

			_, _ = coderdClient.TrackTokenUsage(r.Context(), &proto.TrackTokenUsageRequest{
				MsgId:        msg.ID,
				InputTokens:  msg.Usage.InputTokens,
				OutputTokens: msg.Usage.OutputTokens,
			})

			response.Body = io.NopCloser(bytes.NewReader(body))
			return nil
		}

		response.Body = io.NopCloser(bytes.NewReader(body))
		stream := ant_ssestream.NewStream[anthropic.MessageStreamEventUnion](ant_ssestream.NewDecoder(response), nil)

		var (
			inputToks, outputToks int64
		)

		var msg anthropic.Message
		for stream.Next() {
			event := stream.Current()
			err = msg.Accumulate(event)
			if err != nil {
				// TODO: don't panic.
				panic(err)
			}

			if msg.Usage.InputTokens+msg.Usage.OutputTokens > 0 {
				inputToks = msg.Usage.InputTokens
				outputToks = msg.Usage.OutputTokens
			}
		}

		_, _ = coderdClient.TrackTokenUsage(r.Context(), &proto.TrackTokenUsageRequest{
			MsgId:        msg.ID,
			InputTokens:  inputToks,
			OutputTokens: outputToks,
		})

		response.Body = io.NopCloser(bytes.NewReader(body))

		return nil
	}
	proxy.ServeHTTP(w, r)
}

func (b *Bridge) Serve() error {
	list, err := net.Listen("tcp", b.httpSrv.Addr)
	if err != nil {
		return xerrors.Errorf("listen: %w", err)
	}

	b.addr = list.Addr().String()

	return b.httpSrv.Serve(list) // TODO: TLS.
}

func (b *Bridge) Addr() string {
	return b.addr
}
