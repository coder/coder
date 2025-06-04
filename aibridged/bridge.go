package aibridged

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/coder/freeway/apibridge"
	"github.com/coder/freeway/middleware"
	"github.com/coder/freeway/middleware/logger"
	"github.com/coder/freeway/middleware/provider/anthropic"
	"github.com/coder/freeway/middleware/provider/openai"
	"github.com/coder/freeway/server"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

type Bridge struct {
	httpSrv *http.Server
	addr    string
}

func NewBridge(addr string, store database.Store) *Bridge {

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

	mux := &http.ServeMux{}
	mux.HandleFunc("/v1/chat/completions", proxyOpenAIRequest)

	openAIProvider, err := openai.NewOpenAIProvider("openai", openai.OpenAIProviderConfig{
		BaseURL: "https://api.openai.com/",
		Model:   "gpt-4o",
	})
	if err != nil {
		panic(err) // TODO: don't panic.
	}

	anthropicProvider, err := anthropic.NewAnthropicProvider("anthropic", anthropic.AnthropicProviderConfig{
		Model:            "claude-sonnet-4-0",
		BaseURL:          "https://api.anthropic.com/",
		AnthropicVersion: "2023-06-01",
	})
	if err != nil {
		panic(err) // TODO: don't panic.
	}

	rl, err := logger.NewRequestLogger(nil)
	if err != nil {
		panic(err) // TODO: don't panic.
	}

	pipelines := map[string]server.ModelPipeline{
		"gpt-4o": {
			Provider: openAIProvider,
		},
		"claude-sonnet-4-0": {
			Provider:     anthropicProvider,
			RequestChain: []middleware.RequestMiddleware{rl},
			//ResponseChain: []middleware.ResponseMiddleware{
			//	func(req apibridge.Request, origResp apibridge.Response) (apibridge.Response, error) {
			//		baseID := origResp.ID
			//		modelName := req.Model
			//		messages := req.Messages
			//
			//		//if resp.StopReason == "stop" {
			//		//	event := map[string]any{
			//		//		"prompt": req.Messages,
			//		//		"usage":  resp.Usage,
			//		//	}
			//		//	eventBytes, err := json.Marshal(event)
			//		//	if err != nil {
			//		//		return resp, xerrors.Errorf("marshal event: %w", err)
			//		//	}
			//		//
			//		//	err = store.InsertWormholeEvent(context.TODO(), database.InsertWormholeEventParams{
			//		//		Event:     eventBytes,
			//		//		EventType: "anthropic",
			//		//	})
			//		//	if err != nil {
			//		//		return resp, xerrors.Errorf("wormhole: %w", err)
			//		//	}
			//		//}
			//		//
			//		//return resp, nil
			//
			//		resp := apibridge.Response{
			//			ID:            baseID,
			//			Model:         modelName,
			//			StreamChannel: make(chan apibridge.StreamChunk),
			//		}
			//
			//		go func() {
			//			defer close(resp.StreamChannel)
			//
			//			var allParts []string
			//			for _, message := range messages {
			//				text := extractMessageText(message.Content)
			//				if text == "" {
			//					continue
			//				}
			//				rolePrefix := "unknown message: "
			//				switch message.Role {
			//				case apibridge.RoleSystem:
			//					rolePrefix = "system message: "
			//				case apibridge.RoleUser:
			//					rolePrefix = "user message: "
			//				case apibridge.RoleAssistant: // Should not happen in input, but handle
			//					rolePrefix = "assistant message: "
			//				}
			//				allParts = append(allParts, rolePrefix+text)
			//			}
			//
			//			if len(allParts) == 0 {
			//				// If no content, send only a final chunk with finish reason
			//				finalChunk := apibridge.StreamChunk{
			//					ID:    resp.ID,
			//					Model: resp.Model,
			//					Choices: []apibridge.StreamChoice{
			//						{
			//							Index:        0,
			//							Delta:        apibridge.StreamChoiceDelta{}, // Empty delta
			//							FinishReason: "stop",
			//						},
			//					},
			//					Usage: &apibridge.Usage{}, // Empty usage
			//				}
			//				resp.StreamChannel <- finalChunk
			//
			//				// Send the IsDone chunk for compatibility with server handlers
			//				// This is needed for the OpenAI handler to send the [DONE] marker
			//				doneChunk := apibridge.StreamChunk{
			//					ID:     resp.ID,
			//					Model:  resp.Model,
			//					IsDone: true,
			//				}
			//				resp.StreamChannel <- doneChunk
			//				return
			//			}
			//
			//			// Send each part as a separate chunk - consistent behavior
			//			for _, part := range allParts {
			//				contentChunk := apibridge.StreamChunk{
			//					ID:    resp.ID,
			//					Model: resp.Model,
			//					Choices: []apibridge.StreamChoice{
			//						{
			//							Index: 0,
			//							Delta: apibridge.StreamChoiceDelta{
			//								Role:    apibridge.RoleAssistant,
			//								Content: []apibridge.ContentPart{{Type: apibridge.ContentTypeText, Text: part}},
			//							},
			//						},
			//					},
			//				}
			//				resp.StreamChannel <- contentChunk
			//			}
			//
			//			// Send the final chunk with finish_reason
			//			finalChunk := apibridge.StreamChunk{
			//				ID:    resp.ID,
			//				Model: resp.Model,
			//				Choices: []apibridge.StreamChoice{
			//					{
			//						Index:        0,
			//						Delta:        apibridge.StreamChoiceDelta{}, // Empty delta for final chunk
			//						FinishReason: "stop",
			//					},
			//				},
			//				Usage: &apibridge.Usage{}, // Empty usage
			//			}
			//			resp.StreamChannel <- finalChunk
			//
			//			// Send the IsDone chunk for compatibility with server handlers
			//			// This is needed for the OpenAI handler to send the [DONE] marker
			//			doneChunk := apibridge.StreamChunk{
			//				ID:     resp.ID,
			//				Model:  resp.Model,
			//				IsDone: true,
			//			}
			//			resp.StreamChannel <- doneChunk
			//		}()
			//
			//		return resp, nil
			//	},
			//},
		},
	}

	mux.HandleFunc("/v1/messages", server.RouteAnthropicMessages(pipelines))

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
		// TODO: other settings.
	}

	return &Bridge{httpSrv: srv}
}

func extractMessageText(content []apibridge.ContentPart) string {
	var parts []string
	for _, cp := range content {
		if cp.Type == apibridge.ContentTypeText && cp.Text != "" {
			parts = append(parts, cp.Text)
		}
	}
	return strings.Join(parts, " ")
}

func proxyOpenAIRequest(w http.ResponseWriter, r *http.Request) {
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

func proxyAnthropicRequest(w http.ResponseWriter, r *http.Request) {
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

		fmt.Printf("Proxying %s request to: %s\n", req.Method, req.URL.String())
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		fmt.Println("response", response.ContentLength, response.Status)

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
