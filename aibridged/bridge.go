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
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	ant_ssestream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	ant_constant "github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/google/uuid"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared/constant"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/codersdk"

	"github.com/invopop/jsonschema"
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

	body, err := io.ReadAll(r.Body)
	if err != nil {
		b.logger.Error(r.Context(), "failed to read body", slog.Error(err))
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	var in ChatCompletionNewParamsWrapper
	if err = json.Unmarshal(body, &in); err != nil {
		b.logger.Error(r.Context(), "failed to unmarshal request", slog.Error(err))
		http.Error(w, "failed to unmarshal request", http.StatusInternalServerError)
		return
	}

	prompt, err := in.LastUserPrompt() // TODO: error handling.
	if prompt != nil {
		if _, err = coderdClient.TrackUserPrompts(ctx, &proto.TrackUserPromptsRequest{
			Model:  in.Model,
			Prompt: *prompt,
		}); err != nil {
			b.logger.Error(r.Context(), "failed to track user prompt", slog.Error(err))
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

		es := newEventStream(openAIEventStream)

		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			defer func() {
				if err := es.Close(streamCtx); err != nil {
					b.logger.Error(ctx, "error closing stream", slog.Error(err), slog.F("sessionID", sessionID))
				}
			}()

			BasicSSESender(streamCtx, sessionID, es, b.logger.Named("sse-sender")).ServeHTTP(w, r)
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

					if err := es.TrySend(streamCtx, toolChunk); err != nil {
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

					if err := es.TrySend(streamCtx, finishChunk, "choices[].delta.content"); err != nil {
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
				if err := es.TrySend(streamCtx, chunk); err != nil {
					b.logger.Error(ctx, "failed to send reflected chunk", slog.Error(err))
				}
			}
		}

		if err := es.Close(streamCtx); err != nil {
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

func (b *Bridge) proxyAnthropicRequest(w http.ResponseWriter, r *http.Request) {
	sessionID := uuid.New()
	_, _ = fmt.Fprintf(os.Stderr, "[%s] new chat session started\n\n", sessionID)
	defer func() {
		_, _ = fmt.Fprintf(os.Stderr, "[%s] chat session ended\n\n", sessionID)
	}()

	//out, _ := httputil.DumpRequest(r, true)
	//fmt.Printf("\n\nREQUEST: %s\n\n", out)

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

	useBeta := r.URL.Query().Get("beta") == "true"
	if !useBeta {
		b.logger.Warn(r.Context(), "non-beta API requested, using beta instead", slog.F("url", r.URL.String()))
		useBeta = true
		//http.Error(w, "only beta API supported", http.StatusInternalServerError)
		//return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		b.logger.Error(r.Context(), "failed to read body", slog.Error(err))
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	//var in streamer
	//if useBeta {
	var in BetaMessageNewParamsWrapper
	//} else {
	//	in = &MessageNewParamsWrapper{}
	//}

	if err = json.Unmarshal(body, &in); err != nil {
		b.logger.Error(r.Context(), "failed to unmarshal request", slog.Error(err))
		http.Error(w, "failed to unmarshal request", http.StatusInternalServerError)
		return
	}

	toolParams := []anthropic.BetaToolParam{
		{
			Name:        "get_coordinates",
			Description: anthropic.String("Accepts a place as an address, then returns the latitude and longitude coordinates."),
			InputSchema: GetCoordinatesInputSchema,
		},
	}
	tools := make([]anthropic.BetaToolUnionParam, len(toolParams))
	for i, toolParam := range toolParams {
		tools[i] = anthropic.BetaToolUnionParam{OfTool: &toolParam}
	}
	in.Tools = tools

	// Claude Code uses the 3.5 Haiku model to do autocomplete and other small tasks. (see ANTHROPIC_SMALL_FAST_MODEL).
	isSmallFastModel := strings.Contains(string(in.Model), "3-5-haiku")

	// Find the most recent user message and track the prompt.
	if !isSmallFastModel {
		prompt, err := in.LastUserPrompt() // TODO: error handling.
		if prompt != nil {
			if _, err = coderdClient.TrackUserPrompts(ctx, &proto.TrackUserPromptsRequest{
				Prompt: *prompt,
				Model:  string(in.Model),
			}); err != nil {
				b.logger.Error(r.Context(), "failed to track user prompt", slog.Error(err))
			}
		}
	}

	// looks up API key with os.LookupEnv("ANTHROPIC_API_KEY")
	client := anthropic.NewClient()
	if !in.UseStreaming() {
		msg, err := client.Beta.Messages.New(ctx, in.BetaMessageNewParams)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		// TODO: these figures don't seem to exactly match what Claude Code reports. Find the source of inconsistency!
		_, err = coderdClient.TrackTokenUsage(ctx, &proto.TrackTokenUsageRequest{
			MsgId:        msg.ID,
			Model:        string(msg.Model),
			InputTokens:  msg.Usage.InputTokens,
			OutputTokens: msg.Usage.OutputTokens,
			Other: map[string]int64{
				"web_search_requests":      msg.Usage.ServerToolUse.WebSearchRequests,
				"cache_creation_input":     msg.Usage.CacheCreationInputTokens,
				"cache_read_input":         msg.Usage.CacheReadInputTokens,
				"cache_ephemeral_1h_input": msg.Usage.CacheCreation.Ephemeral1hInputTokens,
				"cache_ephemeral_5m_input": msg.Usage.CacheCreation.Ephemeral5mInputTokens,
			},
		})
		if err != nil {
			b.logger.Error(ctx, "failed to track usage", slog.Error(err))
		}

		out := []byte(msg.RawJSON())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
		return
	}

	streamCtx, streamCancel := context.WithCancelCause(ctx)
	defer streamCancel(xerrors.New("deferred"))

	es := newEventStream(anthropicEventStream)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer func() {
			if err := es.Close(streamCtx); err != nil {
				b.logger.Error(ctx, "error closing stream", slog.Error(err), slog.F("sessionID", sessionID))
			}
		}()

		BasicSSESender(streamCtx, sessionID, es, b.logger.Named("sse-sender")).ServeHTTP(w, r)
	}()

	stream := client.Beta.Messages.NewStreaming(streamCtx, in.BetaMessageNewParams)

	var foundToolCall bool

	var events []anthropic.BetaRawMessageStreamEventUnion
	var message anthropic.BetaMessage
	for stream.Next() {
		event := stream.Current()
		events = append(events, event)

		if err := message.Accumulate(event); err != nil {
			b.logger.Error(ctx, "failed to accumulate streaming events", slog.Error(err), slog.F("event", event), slog.F("msg", message.RawJSON()))
			http.Error(w, "failed to proxy request", http.StatusInternalServerError)
			return
		}

		// [zero] {"type":"content_block_stop"}
		//[zero] {"content_block":{"id":"toolu_015YCpDbjWuSbcKGfDRWR1bD","name":"get_coordinates","type":"tool_use"},"index":1,"type":"content_block_start"}
		//[zero] {"delta":{"type":"input_json_delta"},"index":1,"type":"content_block_delta"}

		switch e := event.AsAny().(type) {
		case anthropic.BetaRawContentBlockStartEvent:
			switch e.ContentBlock.AsAny().(type) {
			case anthropic.BetaToolUseBlock:
				foundToolCall = true // Ensure no more events get sent after this point since our injected tool needs to be called.
				// TODO: ensure ONLY our injected tools cause this.
			}
		}

		if !foundToolCall {
			if err := es.TrySend(streamCtx, event); err != nil {
				b.logger.Error(ctx, "failed to send event", slog.Error(err))
			}
		} else {
			fmt.Printf("[ignored, tool call found] %s\n", event.RawJSON())
		}
	}

	if foundToolCall {
		for _, c := range message.Content {
			switch c.AsAny().(type) {
			case anthropic.BetaToolUseBlock:
				fn := c.AsToolUse().Name
				//input := c.AsToolUse().Input

				var input GetCoordinatesInput
				raw := c.Input
				err = json.Unmarshal(raw, &input)
				if err != nil {
					b.logger.Error(ctx, "failed to send event", slog.Error(err))
					goto outer
				}

				fmt.Printf("[tool] %s %+v\n", fn, input)
				resp := GetCoordinates(input.Location)
				out := fmt.Sprintf("The latitude is %.2f and longitude is %.2f.", resp.Lat, resp.Long)

				_, err = coderdClient.TrackToolUse(streamCtx, &proto.TrackToolUseRequest{
					Model: string(message.Model),
					Input: map[string]string{
						"location": input.Location,
					},
					Tool: fn,
				})
				if err != nil {
					b.logger.Error(ctx, "failed to track usage", slog.Error(err))
				}

				// Start content block
				var textType ant_constant.Text
				if err := es.TrySend(streamCtx, anthropic.BetaRawMessageStreamEventUnion{
					Type:  string(ant_constant.ValueOf[ant_constant.ContentBlockStart]()),
					Index: 0, // TODO: which index to use?
					ContentBlock: anthropic.BetaRawContentBlockStartEventContentBlockUnion{
						Type: string(textType.Default()),
					},
				}); err != nil {
					b.logger.Error(ctx, "failed to send content block start event", slog.Error(err))
				}

				// Send the tool result
				if err := es.TrySend(streamCtx, anthropic.BetaRawMessageStreamEventUnion{
					Type:  string(ant_constant.ValueOf[ant_constant.ContentBlockDelta]()),
					Index: 0, // TODO: which index to use?
					Delta: anthropic.BetaRawMessageStreamEventUnionDelta{
						Type: string(ant_constant.ValueOf[ant_constant.TextDelta]()),
						Text: out,
					},
				}); err != nil {
					b.logger.Error(ctx, "failed to send content block delta event", slog.Error(err))
				}

				// Stop content block
				if err := es.TrySend(streamCtx, anthropic.BetaRawMessageStreamEventUnion{
					Type:  string(ant_constant.ValueOf[ant_constant.ContentBlockStop]()),
					Index: 0, // TODO: which index to use?
				}); err != nil {
					b.logger.Error(ctx, "failed to send content block stop event", slog.Error(err))
				}
			}
		}
	}

outer:
	// TODO: these figures don't seem to exactly match what Claude Code reports. Find the source of inconsistency!
	_, err = coderdClient.TrackTokenUsage(streamCtx, &proto.TrackTokenUsageRequest{
		MsgId:        message.ID,
		Model:        string(message.Model),
		InputTokens:  message.Usage.InputTokens,
		OutputTokens: message.Usage.OutputTokens,
		Other: map[string]int64{
			"web_search_requests":      message.Usage.ServerToolUse.WebSearchRequests,
			"cache_creation_input":     message.Usage.CacheCreationInputTokens,
			"cache_read_input":         message.Usage.CacheReadInputTokens,
			"cache_ephemeral_1h_input": message.Usage.CacheCreation.Ephemeral1hInputTokens,
			"cache_ephemeral_5m_input": message.Usage.CacheCreation.Ephemeral5mInputTokens,
		},
	})
	if err != nil {
		b.logger.Error(ctx, "failed to track usage", slog.Error(err))
	}

	var streamErr error
	if streamErr = stream.Err(); streamErr != nil {
		http.Error(w, stream.Err().Error(), http.StatusInternalServerError)
	}

	err = es.Close(streamCtx)
	if err != nil {
		b.logger.Error(ctx, "failed to close event stream", slog.Error(err))
	}

	wg.Wait()

	// Ensure we flush all the remaining data before ending.
	flush(w)

	if err != nil || streamErr != nil {
		streamCancel(xerrors.Errorf("stream err: %w", err))
	} else {
		streamCancel(xerrors.New("gracefully done"))
	}

	select {
	case <-streamCtx.Done():
	}
}

type GetCoordinatesInput struct {
	Location string `json:"location" jsonschema_description:"The location to look up."`
}

var GetCoordinatesInputSchema = GenerateSchema[GetCoordinatesInput]()

type GetCoordinateResponse struct {
	Long float64 `json:"long"`
	Lat  float64 `json:"lat"`
}

func GetCoordinates(location string) GetCoordinateResponse {
	return GetCoordinateResponse{
		Long: -122.4194,
		Lat:  37.7749,
	}
}

func GenerateSchema[T any]() anthropic.BetaToolInputSchemaParam {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T

	schema := reflector.Reflect(v)

	return anthropic.BetaToolInputSchemaParam{
		Properties: schema.Properties,
	}
}

func (b *Bridge) proxyAnthropicRequestPrev(w http.ResponseWriter, r *http.Request) {
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
