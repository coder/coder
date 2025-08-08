package aibridged

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/aibridged/proto"
)

// Session describes a (potentially) stateful interaction with an AI provider.
type Session interface {
	Init(logger slog.Logger, tracker Tracker, toolMgr ToolManager) string
	Model() string
	ProcessRequest(w http.ResponseWriter, r *http.Request) error
}

var UnknownRoute = xerrors.New("unknown route")

func NewSessionProcessor(p Provider, logger slog.Logger, client proto.DRPCAIBridgeDaemonClient, tools ToolRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, err := p.CreateSession(w, r, tools)
		if err != nil {
			logger.Error(r.Context(), "failed to create session", slog.Error(err), slog.F("path", r.URL.Path))
			http.Error(w, fmt.Sprintf("failed to create %q session", r.URL.Path), http.StatusInternalServerError)
			return
		}

		sessID := sess.Init(logger, NewDRPCTracker(client), NewInjectedToolManager(tools))
		logger = logger.With(slog.F("route", r.URL.Path), slog.F("provider", p.Identifier()), slog.F("session_id", sessID))

		userID, ok := r.Context().Value(ContextKeyBridgeUserID{}).(uuid.UUID)
		if !ok {
			logger.Error(r.Context(), "missing initiator ID in context")
			http.Error(w, "unable to retrieve initiator", http.StatusInternalServerError)
			return
		}

		_, err = client.StartSession(r.Context(), &proto.StartSessionRequest{
			SessionId:   sessID,
			InitiatorId: userID.String(),
			Provider:    p.Identifier(),
			Model:       sess.Model(),
		})
		if err != nil {
			logger.Error(r.Context(), "failed to start session", slog.Error(err))
			http.Error(w, "failed to start session", http.StatusInternalServerError)
			return
		}

		logger.Debug(context.Background(), "started session")

		if err := sess.ProcessRequest(w, r); err != nil {
			logger.Error(r.Context(), "session execution failed", slog.Error(err))
		}
	}
}
