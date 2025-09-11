package aibridged

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/aibridge"
	"github.com/coder/coder/v2/aibridged/proto"
)

var _ http.Handler = &Server{}

// bridgeAIRequest handles requests destined for an upstream AI provider; aibridged intercepts these requests
// and applies a governance layer.
//
// See also: aibridged/middleware.go.
func (s *Server) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
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
		// RequestID:   httpmw.RequestID(r), // TODO: ?
	})

	if err != nil {
		logger.Error(ctx, "failed to handle request", slog.Error(err))
		http.Error(rw, "failed to handle request", http.StatusInternalServerError)
		return
	}

	handler.ServeHTTP(rw, r)
}
