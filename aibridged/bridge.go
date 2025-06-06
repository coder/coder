package aibridged

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/charmbracelet/log"
	"github.com/openai/openai-go"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridged/proto"
)

type Bridge struct {
	httpSrv  *http.Server
	addr     string
	clientFn func() (proto.DRPCAIBridgeDaemonClient, bool)
}

func NewBridge(addr string, clientFn func() (proto.DRPCAIBridgeDaemonClient, bool)) *Bridge {
	// TODO: remove this.
	{
		handler := log.NewWithOptions(os.Stderr, log.Options{
			ReportCaller:    true, // Enable caller reporting for debuggability
			ReportTimestamp: true,
			TimeFormat:      time.TimeOnly,
		})
		handler.SetLevel(log.DebugLevel)
		slog.SetDefault(slog.New(handler))
	}

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

	return &bridge
}

func (b *Bridge) proxyOpenAIRequest(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		// TODO: error handling.
		panic(err)
		return
	}
	r.Body.Close()

	var msg openai.ChatCompletionNewParams
	err = json.Unmarshal(body, &msg)
	if err != nil {
		// TODO: error handling.
		panic(err)
		return
	}

	fmt.Println(msg)

	target, err := url.Parse("https://api.openai.com")
	if err != nil {
		http.Error(w, "failed to parse OpenAI URL", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// Add OpenAI-specific headers
		if strings.TrimSpace(req.Header.Get("Authorization")) == "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("OPENAI_API_KEY")))
		}

		if req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}

		req.Host = target.Host
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host

		fmt.Printf("Proxying %s request to: %s\n", req.Method, req.URL.String())
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
		stream := ssestream.NewStream[anthropic.MessageStreamEventUnion](ssestream.NewDecoder(response), nil)

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
