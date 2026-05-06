package agentchat

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
)

type chatContextKey struct{}

// Context carries the chat identity associated with an agent request.
type Context struct {
	ID          uuid.UUID
	AncestorIDs []uuid.UUID
}

// FromContext returns the chat identity stored on the context.
func FromContext(ctx context.Context) (Context, bool) {
	chatCtx, ok := ctx.Value(chatContextKey{}).(Context)
	if !ok || chatCtx.ID == uuid.Nil {
		return Context{}, false
	}
	return chatCtx, true
}

// WithContext stores chat identity on the context for downstream logs.
func WithContext(ctx context.Context, chatID uuid.UUID, ancestorIDs []uuid.UUID) context.Context {
	if chatID == uuid.Nil {
		return ctx
	}
	ancestors := make([]uuid.UUID, len(ancestorIDs))
	copy(ancestors, ancestorIDs)
	return context.WithValue(ctx, chatContextKey{}, Context{
		ID:          chatID,
		AncestorIDs: ancestors,
	})
}

// Fields returns structured log fields for the chat identity on ctx.
func Fields(ctx context.Context) []slog.Field {
	chatCtx, ok := FromContext(ctx)
	if !ok {
		return nil
	}
	return chatFields(chatCtx.ID, chatCtx.AncestorIDs)
}

// Middleware tags agent logs for requests that originate from
// chatd. Agent log lines emitted while serving a request with Coder-Chat-Id,
// or by background work started by such a request, should include chat_id.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		chatID, ancestorIDs, ok := ExtractContext(r)
		if !ok {
			next.ServeHTTP(rw, r)
			return
		}

		fields := chatFields(chatID, ancestorIDs)
		if requestLogger := loggermw.RequestLoggerFromContext(r.Context()); requestLogger != nil {
			requestLogger.WithFields(fields...)
		}

		ctx := WithContext(r.Context(), chatID, ancestorIDs)
		next.ServeHTTP(rw, r.WithContext(ctx))
	})
}

func chatFields(chatID uuid.UUID, ancestorIDs []uuid.UUID) []slog.Field {
	fields := []slog.Field{slog.F("chat_id", chatID.String())}
	if len(ancestorIDs) == 0 {
		return fields
	}

	ancestors := make([]string, 0, len(ancestorIDs))
	for _, id := range ancestorIDs {
		ancestors = append(ancestors, id.String())
	}
	return append(fields, slog.F("ancestor_chat_ids", ancestors))
}
