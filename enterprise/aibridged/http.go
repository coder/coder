package aibridged

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/aibridge"
	"github.com/coder/coder/v2/enterprise/aibridged/proto"
)

var _ http.Handler = &Server{}

var (
	ErrNoAuthKey             = xerrors.New("no authentication key provided")
	ErrConnect               = xerrors.New("could not connect to coderd")
	ErrUnauthorized          = xerrors.New("unauthorized")
	ErrAcquireRequestHandler = xerrors.New("failed to acquire request handler")
)

// ServeHTTP is the entrypoint for requests which will be intercepted by AI Bridge.
// This function will validate that the given API key may be used to perform the request.
//
// An [aibridge.RequestBridge] instance is acquired from a pool based on the API key's
// owner (referred to as the "initiator"); this instance is responsible for the
// AI Bridge-specific handling of the request.
//
// A [DRPCClient] is provided to the [aibridge.RequestBridge] instance so that data can
// be passed up to a [DRPCServer] for persistence.
func (s *Server) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	logger := s.logger.With(slog.F("path", r.URL.Path))

	key := strings.TrimSpace(ExtractAuthToken(r.Header))
	if key == "" {
		logger.Warn(ctx, "no auth key provided")
		http.Error(rw, ErrNoAuthKey.Error(), http.StatusBadRequest)
		return
	}

	client, err := s.Client()
	if err != nil {
		logger.Warn(ctx, "failed to connect to coderd", slog.Error(err))
		http.Error(rw, ErrConnect.Error(), http.StatusServiceUnavailable)
		return
	}

	resp, err := client.IsAuthorized(ctx, &proto.IsAuthorizedRequest{Key: key})
	if err != nil {
		logger.Warn(ctx, "key authorization check failed", slog.Error(err))
		http.Error(rw, ErrUnauthorized.Error(), http.StatusForbidden)
		return
	}

	// Rewire request context to include actor.
	r = r.WithContext(aibridge.AsActor(ctx, resp.GetOwnerId(), nil))

	id, err := uuid.Parse(resp.GetOwnerId())
	if err != nil {
		logger.Warn(ctx, "failed to parse user ID", slog.Error(err), slog.F("id", resp.GetOwnerId()))
		http.Error(rw, ErrUnauthorized.Error(), http.StatusForbidden)
		return
	}

	handler, err := s.GetRequestHandler(ctx, Request{
		SessionKey:  key,
		APIKeyID:    resp.ApiKeyId,
		InitiatorID: id,
	})
	if err != nil {
		logger.Warn(ctx, "failed to acquire request handler", slog.Error(err))
		http.Error(rw, ErrAcquireRequestHandler.Error(), http.StatusInternalServerError)
		return
	}

	handler.ServeHTTP(rw, r)
}

// ExtractAuthToken extracts authorization token from HTTP request using multiple sources.
// These sources represent the different ways clients authenticate against AI providers.
// It checks the Authorization header (Bearer token) and X-Api-Key header.
// If neither are present, an empty string is returned.
func ExtractAuthToken(header http.Header) string {
	if auth := strings.TrimSpace(header.Get("Authorization")); auth != "" {
		fields := strings.Fields(auth)
		if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
			return fields[1]
		}
	}
	if apiKey := strings.TrimSpace(header.Get("X-Api-Key")); apiKey != "" {
		return apiKey
	}
	return ""
}
