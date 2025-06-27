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
	"github.com/google/uuid"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
	openai_ssestream "github.com/openai/openai-go/packages/ssestream"
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
	// mux.HandleFunc("/v1/chat/completions", bridge.proxyOpenAIRequestPrev)
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

// type SSERoundTripper struct {
//	transport http.RoundTripper
//}
//
// func (s *SSERoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
//	// Use default transport if none specified
//	transport := s.transport
//	if transport == nil {
//		transport = &http.Transport{
//			DisableCompression:    true,
//			ResponseHeaderTimeout: 0, // No timeout for SSE
//			IdleConnTimeout:       300 * time.Second,
//		}
//	}
//
//	// Modify request for SSE
//	req.Header.Set("Cache-Control", "no-cache")
//	req.Header.Set("Accept", "text/event-stream")
//
//	resp, err := transport.RoundTrip(req)
//	if err != nil {
//		return resp, err
//	}
//
//	resp.Body = wrapResponseBody(resp.Body)
//
//	//var buf bytes.Buffer
//	//teeReader := io.TeeReader(resp.Body, &buf)
//	////out, err := io.ReadAll(teeReader)
//	////if err != nil {
//	////	return nil, xerrors.Errorf("intercept stream: %w", err)
//	////}
//	//
//	//newResp := &http.Response{
//	//	Body:   io.NopCloser(bytes.NewBuffer(buf.Bytes())),
//	//	Header: resp.Header,
//	//}
//	//
//	//stream := openai_ssestream.NewStream[openai.ChatCompletionChunk](openai_ssestream.NewDecoder(newResp), nil)
//	//
//	//var msg openai.ChatCompletionAccumulator
//	//for stream.Next() {
//	//	chunk := stream.Current()
//	//	msg.AddChunk(chunk)
//	//
//	//	fmt.Println(chunk)
//	//}
//
//	return resp, err
//}
//
// func wrapResponseBody(body io.ReadCloser) io.ReadCloser {
//	pr, pw := io.Pipe()
//	go func() {
//		defer pw.Close()
//		defer body.Close()
//
//		var buf bytes.Buffer
//		teeReader := io.TeeReader(pr, &buf)
//
//		// Read the entire stream first
//		streamData, err := io.ReadAll(teeReader)
//		if err != nil {
//			return
//		}
//
//		// Write the original data to the pipe for the client
//		go func() {
//			defer pw.Close()
//			pw.Write(streamData)
//		}()
//	}()
//
//	return pr
//}

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

	// Establish SSE stream which we will connect to requesting client.
	// clientStream := NewSSEStream(eventsCh, b.logger.Named("sse-stream"))

	coderdClient, ok := b.clientFn()
	if !ok {
		// TODO: log issue.
		http.Error(w, "could not acquire coderd client", http.StatusInternalServerError)
		return
	}
	_ = coderdClient

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

					// type noOmitChoice struct {
					//	openai.ChatCompletionChunkChoice
					//
					//	Delta openai.ChatCompletionChunkChoiceDelta `json:"delta,required,no_omit"`
					//}
					//
					//type noOmitChunk struct {
					//	openai.ChatCompletionChunk
					//	Choices []noOmitChoice `json:"choices,required"`
					//}
					//
					//finishChunk := noOmitChunk{
					//	ChatCompletionChunk: openai.ChatCompletionChunk{
					//		ID:                acc.ID,
					//		Model:             toolRes.Model,
					//		ServiceTier:       openai.ChatCompletionChunkServiceTier(toolRes.ServiceTier),
					//		Created:           time.Now().Unix(),
					//		SystemFingerprint: toolRes.SystemFingerprint,
					//		Usage:             toolRes.Usage,
					//		Object:            constant.ValueOf[constant.ChatCompletionChunk](),
					//	},
					//	Choices: []noOmitChoice{
					//		{
					//			ChatCompletionChunkChoice: openai.ChatCompletionChunkChoice{
					//				FinishReason: string(openai.CompletionChoiceFinishReasonStop),
					//			},
					//			Delta: openai.ChatCompletionChunkChoiceDelta{
					//				//Role:    "assistant",
					//				Content: "",
					//			},
					//		},
					//	},
					//}

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

func (b *Bridge) proxyOpenAIRequestPrev(w http.ResponseWriter, r *http.Request) {
	coderdClient, ok := b.clientFn()
	if !ok {
		// TODO: log issue.
		http.Error(w, "could not acquire coderd client", http.StatusInternalServerError)
		return
	}

	var acc openai.ChatCompletionAccumulator
	proxy, err := NewSSEProxyWithConfig(ProxyConfig{
		OpenAISession: NewOpenAISession(),
		Target:        b.openAITarget(),
		ModifyRequest: func(req *http.Request) {
			var in ChatCompletionNewParamsWrapper

			body, err := io.ReadAll(req.Body)
			if err != nil {
				b.logger.Error(req.Context(), "failed to read body", slog.Error(err))
				http.Error(w, "failed to read body", http.StatusInternalServerError)
				return
			}

			if err = json.Unmarshal(body, &in); err != nil {
				b.logger.Error(req.Context(), "failed to unmarshal request", slog.Error(err))
				http.Error(w, "failed to unmarshal request", http.StatusInternalServerError)
				return
			}

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

			newBody, err := json.Marshal(in)
			if err != nil {
				b.logger.Error(req.Context(), "failed to marshal request", slog.Error(err))
				http.Error(w, "failed to marshal request", http.StatusInternalServerError)
				return
			}

			req.Body = io.NopCloser(bytes.NewReader(newBody))
		},
		RequestInterceptFunc: func(req *http.Request, body []byte) error {
			var msg ChatCompletionNewParamsWrapper
			err := json.NewDecoder(bytes.NewReader(body)).Decode(&msg)
			if err != nil {
				http.Error(w, "could not unmarshal request body", http.StatusBadRequest)
				return xerrors.Errorf("unmarshal request body: %w", err)
			}
			// TODO: robustness
			if len(msg.Messages) > 0 {
				latest := msg.Messages[len(msg.Messages)-1]
				if latest.OfUser != nil {
					if latest.OfUser.Content.OfString.String() != "" {
						_, _ = coderdClient.TrackUserPrompts(r.Context(), &proto.TrackUserPromptsRequest{
							Prompt: strings.TrimSpace(latest.OfUser.Content.OfString.String()),
						})
					}
				}
			}
			return nil
		},
		ResponseInterceptFunc: func(session *OpenAISession, data []byte, isStreaming bool) ([][]byte, bool, error) {
			b.logger.Info(r.Context(), "openai response received", slog.F("data", data), slog.F("streaming", isStreaming))

			if !isStreaming {
				return nil, true, nil
			}

			response := &http.Response{
				Body: io.NopCloser(bytes.NewReader(data)),
			}
			stream := openai_ssestream.NewStream[openai.ChatCompletionChunk](openai_ssestream.NewDecoder(response), nil)

			var (
				inputToks, outputToks int64
			)
			for stream.Next() {
				chunk := stream.Current()

				acc.AddChunk(chunk)
				b.logger.Info(r.Context(), "openai chunk", slog.F("msgID", acc.ID), slog.F("contents", fmt.Sprintf("%+v", acc)))

				if acc.Usage.PromptTokens+acc.Usage.CompletionTokens > 0 {
					inputToks = acc.Usage.PromptTokens
					outputToks = acc.Usage.CompletionTokens
				}

				var foundToolCallDelta bool
				for _, c := range chunk.Choices {
					for range c.Delta.ToolCalls {
						foundToolCallDelta = true

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

				if foundToolCallDelta {
					// Don't reflect these events back to client since they contain tool calls.
					return nil, false, nil
				}
			}
			if err := stream.Err(); err != nil {
				panic(err)
			}

			if inputToks+outputToks > 0 {
				_, _ = coderdClient.TrackTokenUsage(r.Context(), &proto.TrackTokenUsageRequest{
					MsgId:        acc.ID,
					InputTokens:  inputToks,
					OutputTokens: outputToks,
				})
			}

			if len(acc.Choices) < 0 {
				return nil, true, nil
			}

			var extra [][]byte
			for idx, t := range session.toolCallsRequired {
				// TODO: locking.
				// TODO: index check.
				session.toolCallsState[idx] = OpenAIToolCallInProgress

				fmt.Printf("EXEC TOOL! %s with %+v\n", t.funcName, t.args)
				b, _ := json.Marshal(openai.ToolMessage("weather is rainy and cold in cape town today", idx)) // TODO: error handling.
				extra = append(extra, b)

				session.toolCallsState[idx] = OpenAIToolCallDone
			}

			return extra, true, nil
		},
	})
	if err != nil {
		b.logger.Error(r.Context(), "failed to create OpenAI proxy", slog.Error(err))
		http.Error(w, "failed to create OpenAI proxy", http.StatusInternalServerError)
		return
	}

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Add OpenAI-specific headers
		if strings.TrimSpace(req.Header.Get("Authorization")) == "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("OPENAI_API_KEY")))
		}
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	proxy.ServeHTTP(w, r)
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
