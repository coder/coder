package agentchat_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMiddlewareAccessLog(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	ancestorID := uuid.New()
	sink := testutil.NewFakeSink(t)
	handler := tracing.StatusWriterMiddleware(loggermw.Logger(sink.Logger(), nil)(
		agentchat.Middleware(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusNoContent)
		})),
	))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(workspacesdk.CoderChatIDHeader, chatID.String())
	req.Header.Set(workspacesdk.CoderAncestorChatIDsHeader, mustMarshalJSON(t, []string{ancestorID.String()}))
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	require.Equal(t, http.StatusNoContent, rw.Code)

	entries := sink.Entries()
	require.Len(t, entries, 1)
	fields := fieldsByName(entries[0].Fields)
	require.Equal(t, chatID.String(), fields["chat_id"])
	require.Equal(t, []string{ancestorID.String()}, fields["ancestor_chat_ids"])
}

func TestMiddlewareWithoutChatHeader(t *testing.T) {
	t.Parallel()

	sink := testutil.NewFakeSink(t)
	handler := tracing.StatusWriterMiddleware(loggermw.Logger(sink.Logger(), nil)(
		agentchat.Middleware(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusNoContent)
		})),
	))

	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/test", nil))
	require.Equal(t, http.StatusNoContent, rw.Code)

	entries := sink.Entries()
	require.Len(t, entries, 1)
	fields := fieldsByName(entries[0].Fields)
	require.NotContains(t, fields, "chat_id")
	require.NotContains(t, fields, "ancestor_chat_ids")
}

func TestMiddlewareContextFields(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	sink := testutil.NewFakeSink(t)
	handler := tracing.StatusWriterMiddleware(loggermw.Logger(sink.Logger(), nil)(
		agentchat.Middleware(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			sink.Logger().With(agentchat.Fields(r.Context())...).Info(r.Context(), "handler log")
			rw.WriteHeader(http.StatusNoContent)
		})),
	))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(workspacesdk.CoderChatIDHeader, chatID.String())
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	require.Equal(t, http.StatusNoContent, rw.Code)

	entries := sink.Entries()
	require.Len(t, entries, 2)
	for _, entry := range entries {
		if entry.Message != "handler log" {
			continue
		}
		fields := fieldsByName(entry.Fields)
		require.Equal(t, chatID.String(), fields["chat_id"])
		return
	}
	t.Fatal("handler log entry not found")
}

func fieldsByName(fields []slog.Field) map[string]any {
	byName := make(map[string]any, len(fields))
	for _, field := range fields {
		byName[field.Name] = field.Value
	}
	return byName
}
