package aibridged

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"cdr.dev/slog"
	"github.com/anthropics/anthropic-sdk-go"
	ant_ssestream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
	openai_ssestream "github.com/openai/openai-go/packages/ssestream"
	"github.com/tidwall/gjson"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridged/proto"
)

type Bridge struct {
	httpSrv  *http.Server
	addr     string
	clientFn func() (proto.DRPCAIBridgeDaemonClient, bool)
	logger   slog.Logger
}

func NewBridge(addr string, logger slog.Logger, clientFn func() (proto.DRPCAIBridgeDaemonClient, bool)) *Bridge {
	var bridge Bridge

	mux := &http.ServeMux{}
	mux.HandleFunc("/v1/chat/completions", bridge.proxyOpenAIRequest)
	mux.HandleFunc("/v1/messages", bridge.proxyAnthropicRequest)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
		// TODO: other settings.
	}

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

//type SSERoundTripper struct {
//	transport http.RoundTripper
//}
//
//func (s *SSERoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
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
//func wrapResponseBody(body io.ReadCloser) io.ReadCloser {
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

func (b *Bridge) proxyOpenAIRequest(w http.ResponseWriter, r *http.Request) {
	coderdClient, ok := b.clientFn()
	if !ok {
		// TODO: log issue.
		http.Error(w, "could not acquire coderd client", http.StatusInternalServerError)
		return
	}

	target, err := url.Parse("https://api.openai.com")
	if err != nil {
		http.Error(w, "failed to parse OpenAI URL", http.StatusInternalServerError)
		return
	}

	proxy, err := NewSSEProxyWithConfig(ProxyConfig{
		Target: target,
		RequestInterceptFunc: func(req *http.Request, body []byte) error {
			var msg ChatCompletionNewParamsWrapper
			err = json.NewDecoder(bytes.NewReader(body)).Decode(&msg)
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
		ResponseInterceptFunc: func(data []byte, isStreaming bool) error {
			b.logger.Info(r.Context(), "openai response received", slog.F("data", data), slog.F("streaming", isStreaming))

			if !isStreaming {
				return nil
			}

			response := &http.Response{
				Body: io.NopCloser(bytes.NewReader(data)),
			}
			stream := openai_ssestream.NewStream[openai.ChatCompletionChunk](openai_ssestream.NewDecoder(response), nil)

			var (
				inputToks, outputToks int64
			)
			var msg openai.ChatCompletionAccumulator
			for stream.Next() {
				msg.AddChunk(stream.Current())
				b.logger.Info(r.Context(), "openai chunk", slog.F("msgID", msg.ID), slog.F("contents", fmt.Sprintf("%+v", msg)))

				if msg.Usage.PromptTokens+msg.Usage.CompletionTokens > 0 {
					inputToks = msg.Usage.PromptTokens
					outputToks = msg.Usage.CompletionTokens
				}
			}

			if inputToks+outputToks > 0 {
				_, _ = coderdClient.TrackTokenUsage(r.Context(), &proto.TrackTokenUsageRequest{
					MsgId:        msg.ID,
					InputTokens:  inputToks,
					OutputTokens: outputToks,
				})
			}

			return nil
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
		//Add OpenAI-specific headers
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
