package aibridged

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/aibridge"
	"github.com/coder/coder/v2/aibridged/proto"
)

var _ http.Handler = &server{}

// bridgeAIRequest handles requests destined for an upstream AI provider; aibridged intercepts these requests
// and applies a governance layer.
func (s *server) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	logger := s.logger.With(slog.F("path", r.URL.Path))

	key := strings.TrimSpace(extractAuthToken(r))
	if key == "" {
		logger.Warn(ctx, "no auth key provided")
		http.Error(rw, "no authentication key provided", http.StatusBadRequest)
		return
	}

	client, err := s.Client()
	if err != nil {
		logger.Error(ctx, "failed to connect to coderd", slog.Error(err))
		http.Error(rw, "could not connect to coderd", http.StatusInternalServerError)
		return
	}

	resp, err := client.AuthenticateKey(ctx, &proto.AuthenticateKeyRequest{Key: key})
	if err != nil {
		logger.Error(ctx, "failed to authenticate key", slog.Error(err))
		http.Error(rw, "unauthorized", http.StatusForbidden)
		return
	}

	// Rewire request context to include actor.
	r = r.WithContext(aibridge.AsActor(ctx, resp.GetOwnerId(), nil))

	id, err := uuid.Parse(resp.GetOwnerId())
	if err != nil {
		logger.Error(ctx, "failed to parse user ID", slog.Error(err), slog.F("id", resp.GetOwnerId()))
		http.Error(rw, "unauthorized", http.StatusForbidden)
		return
	}

	handler, err := s.GetRequestHandler(ctx, Request{
		SessionKey:  key,
		InitiatorID: id,
	})
	if err != nil {
		logger.Error(ctx, "failed to handle request", slog.Error(err))
		http.Error(rw, "failed to handle request", http.StatusInternalServerError)
		return
	}

	handler.ServeHTTP(rw, r)
}

// extractAuthToken extracts authorization token from HTTP request using multiple sources.
// These sources represent the different ways clients authenticate against AI providers.
// It checks the Authorization header (Bearer token) and X-Api-Key header.
// If neither are present, an empty string is returned.
func extractAuthToken(r *http.Request) string {
	// 1. Check Authorization header for Bearer token.
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		segs := strings.Split(authHeader, " ")
		if len(segs) > 1 {
			if strings.ToLower(segs[0]) == "bearer" {
				return strings.Join(segs[1:], "")
			}
		}
	}

	// 2. Check X-Api-Key header.
	apiKeyHeader := r.Header.Get("X-Api-Key")
	if apiKeyHeader != "" {
		return apiKeyHeader
	}

	return ""
}
