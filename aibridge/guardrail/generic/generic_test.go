package generic_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/coder/coder/v2/aibridge/guardrail"
	"github.com/coder/coder/v2/aibridge/guardrail/generic"
)

const body = `{"messages":[{"role":"system","content":"be nice"},{"role":"user","content":"my email is a@b.com"}]}`

type capturedReq struct {
	Texts              []string         `json:"texts"`
	StructuredMessages []map[string]any `json:"structured_messages"`
	InputType          string           `json:"input_type"`
	Params             map[string]any   `json:"additional_provider_specific_params"`
}

// mockServer returns a configurable Generic Guardrail API. It records the last
// request and the auth/static headers it received.
func mockServer(t *testing.T, resp map[string]any) (*httptest.Server, *capturedReq, *http.Header) {
	t.Helper()
	var (
		got     capturedReq
		headers http.Header
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/beta/litellm_basic_guardrail_api", r.URL.Path)
		headers = r.Header.Clone()
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)
	return srv, &got, &headers
}

func newReq(t *testing.T) guardrail.Request {
	t.Helper()
	return guardrail.Request{
		Body:         []byte(body),
		Prompt:       guardrail.UserPromptTexts([]byte(body)),
		Conversation: guardrail.ConversationTexts([]byte(body)),
		Identity:     guardrail.Identity{ID: "u1", Username: "alice"},
	}
}

func TestGeneric_Intervened_MasksPositionally(t *testing.T) {
	t.Parallel()
	srv, got, headers := mockServer(t, map[string]any{
		"action": "GUARDRAIL_INTERVENED",
		"texts":  []string{"my email is <REDACTED>"},
	})
	g, err := generic.New(generic.Config{
		Name:    "pillar",
		APIBase: srv.URL,
		Headers: map[string]string{"X-Service": "coder"},
	}, "secret-token")
	require.NoError(t, err)

	res, err := g.Evaluate(context.Background(), newReq(t))
	require.NoError(t, err)
	require.False(t, res.Action == guardrail.ActionBlock)
	require.Len(t, res.Edits, 1)
	require.Equal(t, "messages.1.content", res.Edits[0].Pointer)
	require.Equal(t, "my email is <REDACTED>", res.Edits[0].Value)

	// Default scope is prompt: only the latest user span is sent.
	require.Equal(t, []string{"my email is a@b.com"}, got.Texts)
	require.Equal(t, "request", got.InputType)
	require.Equal(t, "Bearer secret-token", headers.Get("Authorization"))
	require.Equal(t, "coder", headers.Get("X-Service"))
}

func TestGeneric_Blocked(t *testing.T) {
	t.Parallel()
	srv, _, _ := mockServer(t, map[string]any{
		"action":         "BLOCKED",
		"blocked_reason": "prompt injection detected",
	})
	g, err := generic.New(generic.Config{Name: "pillar", APIBase: srv.URL}, "")
	require.NoError(t, err)

	res, err := g.Evaluate(context.Background(), newReq(t))
	require.NoError(t, err)
	require.Equal(t, guardrail.ActionBlock, res.Action)
	require.Equal(t, "prompt injection detected", res.Reason)
}

func TestGeneric_None_NoEdits(t *testing.T) {
	t.Parallel()
	srv, _, _ := mockServer(t, map[string]any{"action": "NONE"})
	g, err := generic.New(generic.Config{Name: "pillar", APIBase: srv.URL}, "")
	require.NoError(t, err)

	res, err := g.Evaluate(context.Background(), newReq(t))
	require.NoError(t, err)
	require.NotEqual(t, guardrail.ActionBlock, res.Action)
	require.Empty(t, res.Edits)
}

func TestGeneric_ConversationScopeSendsAllSpans(t *testing.T) {
	t.Parallel()
	srv, got, _ := mockServer(t, map[string]any{"action": "NONE"})
	g, err := generic.New(generic.Config{
		Name:    "pillar",
		APIBase: srv.URL,
		Scope:   generic.ScopeConversation,
	}, "")
	require.NoError(t, err)

	_, err = g.Evaluate(context.Background(), newReq(t))
	require.NoError(t, err)
	require.Equal(t, []string{"be nice", "my email is a@b.com"}, got.Texts)
	require.Equal(t, "system", got.StructuredMessages[0]["role"])
	require.Equal(t, "user", got.StructuredMessages[1]["role"])
}

func TestGeneric_5xxIsError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	g, err := generic.New(generic.Config{Name: "pillar", APIBase: srv.URL}, "")
	require.NoError(t, err)

	_, err = g.Evaluate(context.Background(), newReq(t))
	require.Error(t, err) // surfaces to the Stage, which applies fail mode
}

func TestGeneric_MaskWriteBackApplies(t *testing.T) {
	t.Parallel()
	// The positional write-back lands on the originating pointer, so the Stage
	// can thread it into the body for downstream members.
	srv, _, _ := mockServer(t, map[string]any{
		"action": "GUARDRAIL_INTERVENED",
		"texts":  []string{"clean"},
	})
	g, err := generic.New(generic.Config{Name: "pillar", APIBase: srv.URL}, "")
	require.NoError(t, err)

	res, err := g.Evaluate(context.Background(), newReq(t))
	require.NoError(t, err)
	require.Len(t, res.Edits, 1)

	out, err := sjson.SetBytes([]byte(body), res.Edits[0].Pointer, res.Edits[0].Value)
	require.NoError(t, err)
	require.Equal(t, "clean", gjson.GetBytes(out, "messages.1.content").String())
}
