package bedrock_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/guardrail"
	"github.com/coder/coder/v2/aibridge/guardrail/bedrock"
)

const body = `{"messages":[{"role":"user","content":"my SSN is 123-45-6789"}]}`

// mockBedrock returns an ApplyGuardrail stand-in that asserts the request was
// SigV4-signed and returns the supplied response.
func mockBedrock(t *testing.T, resp map[string]any) (*httptest.Server, *http.Header) {
	t.Helper()
	var headers http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.Path, "/guardrail/")
		require.Contains(t, r.URL.Path, "/apply")
		headers = r.Header.Clone()
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)
	return srv, &headers
}

func newGuardrail(t *testing.T, endpoint string) *bedrock.Guardrail {
	t.Helper()
	cfg, err := json.Marshal(map[string]string{
		"region":               "us-east-1",
		"guardrail_identifier": "gr-123",
		"guardrail_version":    "DRAFT",
		"endpoint":             endpoint,
	})
	require.NoError(t, err)
	cred, err := json.Marshal(map[string]string{
		"access_key_id":     "AKIDEXAMPLE",
		"secret_access_key": "secret",
	})
	require.NoError(t, err)
	g, err := bedrock.NewFromConfig("bedrock-pii", cfg, string(cred))
	require.NoError(t, err)
	return g
}

func req() guardrail.Request {
	return guardrail.Request{
		Body:   []byte(body),
		Prompt: guardrail.UserPromptTexts([]byte(body)),
	}
}

func TestBedrock_AnonymizeMasksAndSigns(t *testing.T) {
	t.Parallel()
	srv, headers := mockBedrock(t, map[string]any{
		"action":  "GUARDRAIL_INTERVENED",
		"outputs": []map[string]any{{"text": "my SSN is {US_SSN}"}},
		"assessments": []map[string]any{{
			"sensitiveInformationPolicy": map[string]any{
				"piiEntities": []map[string]any{{"type": "US_SSN", "action": "ANONYMIZED"}},
			},
		}},
	})
	g := newGuardrail(t, srv.URL)

	res, err := g.Evaluate(context.Background(), req())
	require.NoError(t, err)
	require.NotEqual(t, guardrail.ActionBlock, res.Action)
	require.Len(t, res.Edits, 1)
	require.Equal(t, "messages.0.content", res.Edits[0].Pointer)
	require.Equal(t, "my SSN is {US_SSN}", res.Edits[0].Value)
	require.Equal(t, map[string]int{"US_SSN": 1}, res.Annotations["pii_entities"])

	// SigV4 signed (not a static bearer token).
	require.True(t, strings.HasPrefix(headers.Get("Authorization"), "AWS4-HMAC-SHA256"),
		"expected SigV4 Authorization, got %q", headers.Get("Authorization"))
	require.NotEmpty(t, headers.Get("X-Amz-Date"))
}

func TestBedrock_ContentFilterBlocks(t *testing.T) {
	t.Parallel()
	// A content-filter assessment with a BLOCKED action is a block, even though
	// the wire action is the same GUARDRAIL_INTERVENED as a mask.
	srv, _ := mockBedrock(t, map[string]any{
		"action":  "GUARDRAIL_INTERVENED",
		"outputs": []map[string]any{{"text": "Sorry, I can't help with that."}},
		"assessments": []map[string]any{{
			"contentPolicy": map[string]any{
				"filters": []map[string]any{{"type": "VIOLENCE", "action": "BLOCKED"}},
			},
		}},
	})
	g := newGuardrail(t, srv.URL)

	res, err := g.Evaluate(context.Background(), req())
	require.NoError(t, err)
	require.Equal(t, guardrail.ActionBlock, res.Action)
	require.Empty(t, res.Edits)
	require.Equal(t, true, res.Annotations["blocked"])
}

func TestBedrock_NoneAllows(t *testing.T) {
	t.Parallel()
	srv, _ := mockBedrock(t, map[string]any{"action": "NONE"})
	g := newGuardrail(t, srv.URL)

	res, err := g.Evaluate(context.Background(), req())
	require.NoError(t, err)
	require.NotEqual(t, guardrail.ActionBlock, res.Action)
	require.Empty(t, res.Edits)
}

func TestBedrock_RequiresCredentialFields(t *testing.T) {
	t.Parallel()
	cfg, err := json.Marshal(map[string]string{"region": "us-east-1"})
	require.NoError(t, err)
	_, err = bedrock.NewFromConfig("bad", cfg, "")
	require.ErrorContains(t, err, "guardrail_identifier required")
}
