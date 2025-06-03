package aibridged

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"cdr.dev/slog"
	"storj.io/drpc/drpcmux"

	"github.com/coder/coder/v2/aibridged/proto"
)

type AIProvider string

const (
	AIProviderOpenAI    AIProvider = "openai"
	AIProviderAnthropic AIProvider = "anthropic"
)

type ProxyConfig struct {
	ReadTimeout time.Duration
}

type HTTPProxy struct {
	logger     slog.Logger
	provider   AIProvider
	config     ProxyConfig
	mux        *drpcmux.Mux
	drpcClient AIServiceClient
}

// NewDRPCProxy creates a new reverse proxy instance.
func NewDRPCProxy(client AIServiceClient, config ProxyConfig) (*HTTPProxy, error) {
	return &HTTPProxy{
		config:     config,
		mux:        drpcmux.New(),
		drpcClient: client,
	}, nil
} // ServeHTTP handles incoming HTTP requests and proxies them to dRPC

func (p *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Read request body
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MiB // TODO: make configurable.
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// TODO: use body
	_ = body

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), p.config.ReadTimeout)
	defer cancel()

	switch p.drpcClient.(type) {
	case *OpenAIAdapter:
		payload := struct {
			messages []struct {
				Role    string `json:"role,omitempty"`
				Content string `json:"content,omitempty"`
			}
			model string
		}{
			messages: []struct {
				Role    string `json:"role,omitempty"`
				Content string `json:"content,omitempty"`
			}{
				{
					Role:    "developer",
					Content: "what is 9/0?",
				},
			},
			model: "gpt-4o",
		}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			// TODO: better error.
			http.Error(w, "failed to marshal payload", http.StatusInternalServerError)
			return
		}

		resp, err := p.drpcClient.SendRequest(ctx, &proto.JSONPayload{
			Content: string(payloadJSON),
		})
		if err != nil {
			return
		}

		// TODO: start SSE
		for {
			respPayload, err := resp.Recv()
			if err != nil {
				if err == io.EOF {
					return
				}
				http.Error(w, "failed to receive payload", http.StatusInternalServerError)
				return
			}

			_, _ = w.Write([]byte(respPayload.Content))
		}

		// Set appropriate headers
		//w.Header().Set("Content-Type", "application/json")

		// Write response
		//if _, err := w.Write([]byte(resp.)); err != nil {
		//	p.logger.Warn(ctx, "failed to write response: %w", err)
		//}
	case *AnthropicAdapter:
	}
}
