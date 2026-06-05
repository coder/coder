package aibridge_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/policy"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// fakeInterceptor is a minimal intercept.Interceptor that writes a 200 response.
type fakeInterceptor struct {
	id    uuid.UUID
	model string
}

func (f *fakeInterceptor) ID() uuid.UUID                                         { return f.id }
func (*fakeInterceptor) Setup(slog.Logger, recorder.Recorder, mcp.ServerProxier) {}
func (f *fakeInterceptor) Model() string                                         { return f.model }
func (*fakeInterceptor) Streaming() bool                                         { return false }
func (*fakeInterceptor) TraceAttributes(*http.Request) []attribute.KeyValue      { return nil }
func (*fakeInterceptor) Credential() intercept.CredentialInfo                    { return intercept.CredentialInfo{} }
func (*fakeInterceptor) CorrelatingToolCallID() *string                          { return nil }
func (*fakeInterceptor) ProcessRequest(w http.ResponseWriter, _ *http.Request) error {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
	return nil
}

type capture struct {
	called  bool
	payload intercept.Payload
}

func mustDecide(t *testing.T, name, module string) *policy.Decide {
	t.Helper()
	d, err := policy.NewDecide(name, module)
	require.NoError(t, err)
	return d
}

// newPolicyBridge builds a bridge whose single bridged route runs preReq and
// records the payload that reaches CreateInterceptor.
func newPolicyBridge(t *testing.T, preReq *policy.Pipeline) (http.Handler, *testutil.MockRecorder, *capture) {
	t.Helper()
	cap := &capture{}
	prov := &testutil.MockProvider{
		NameStr: "mock",
		Bridged: []string{"/v1/messages"},
		InterceptorFunc: func(_ http.ResponseWriter, _ *http.Request, payload intercept.Payload, _ trace.Tracer) (intercept.Interceptor, error) {
			cap.called = true
			cap.payload = payload
			return &fakeInterceptor{id: uuid.New(), model: payload.Model()}, nil
		},
	}
	rec := &testutil.MockRecorder{}
	bridge, err := aibridge.NewRequestBridge(
		t.Context(), []provider.Provider{prov}, rec, nil,
		slogtest.Make(t, nil), nil, bridgeTestTracer,
		aibridge.WithPolicyHooks(nil, preReq),
	)
	require.NoError(t, err)
	return bridge, rec, cap
}

func policyRequest(t *testing.T, bridge http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	ctx := aibridge.AsActor(t.Context(), "user-1", recorder.Metadata{"team": "platform"})
	req := httptest.NewRequest(http.MethodPost, "/mock/v1/messages", strings.NewReader(body)).WithContext(ctx)
	resp := httptest.NewRecorder()
	bridge.ServeHTTP(resp, req)
	return resp
}

func TestBridgePolicy_Block(t *testing.T) {
	t.Parallel()

	pipe, err := policy.NewPipeline(policy.PipelineConfig{
		Decide: []*policy.Decide{mustDecide(t, "model-blocker", `
default verdict := "ALLOW"
verdict := "BLOCK" if input.request.model == "blocked"
`)},
	})
	require.NoError(t, err)
	bridge, _, cap := newPolicyBridge(t, pipe)

	resp := policyRequest(t, bridge, `{"model":"blocked"}`)
	assert.Equal(t, http.StatusForbidden, resp.Code)
	assert.Contains(t, resp.Body.String(), `request blocked by policy "model-blocker"`)
	assert.False(t, cap.called, "interceptor must not be created when blocked")
}

func TestBridgePolicy_FailClosed(t *testing.T) {
	t.Parallel()

	// object.get on a string (input.request.model) is a builtin type error.
	pipe, err := policy.NewPipeline(policy.PipelineConfig{
		Decide: []*policy.Decide{mustDecide(t, "type-error", `
default verdict := "ALLOW"
verdict := "BLOCK" if object.get(input.request.model, "k", "") == ""
`)},
	})
	require.NoError(t, err)
	bridge, _, cap := newPolicyBridge(t, pipe)

	resp := policyRequest(t, bridge, `{"model":"gpt-4o"}`)
	assert.Equal(t, http.StatusForbidden, resp.Code)
	assert.False(t, cap.called)
}

func TestBridgePolicy_Route(t *testing.T) {
	t.Parallel()

	route, err := policy.NewRoute("model-router", `
model := "routed-model" if input.request.model == "original"
`)
	require.NoError(t, err)
	pipe, err := policy.NewPipeline(policy.PipelineConfig{Route: route})
	require.NoError(t, err)
	bridge, rec, cap := newPolicyBridge(t, pipe)

	resp := policyRequest(t, bridge, `{"model":"original"}`)
	require.Equal(t, http.StatusOK, resp.Code)
	require.True(t, cap.called)
	assert.Equal(t, "routed-model", cap.payload.Model(), "route override must reach the interceptor")

	// The model change is recorded under Metadata["modifications"], keyed by the
	// policy name, with the original model.
	interceptions := rec.RecordedInterceptions()
	require.Len(t, interceptions, 1)
	modifications, ok := interceptions[0].Metadata["modifications"].(map[string]any)
	require.True(t, ok, "modifications must be recorded under Metadata")
	mod, ok := modifications["model-router"].(map[string]any)
	require.True(t, ok, "modification must be keyed by policy name")
	assert.Equal(t, "original", mod["original_model"])
}

func TestBridgePolicy_Transform(t *testing.T) {
	t.Parallel()

	tr, err := policy.NewTransform("max-tokens-clamp", `
ceiling := 8192
body := object.union(input.request, {"max_tokens": ceiling}) if object.get(input.request, "max_tokens", 0) > ceiling
`)
	require.NoError(t, err)
	pipe, err := policy.NewPipeline(policy.PipelineConfig{Transform: []*policy.Transform{tr}})
	require.NoError(t, err)
	bridge, _, cap := newPolicyBridge(t, pipe)

	resp := policyRequest(t, bridge, `{"model":"gpt-4o","max_tokens":1000000}`)
	require.Equal(t, http.StatusOK, resp.Code)
	require.True(t, cap.called)

	var got map[string]any
	require.NoError(t, json.Unmarshal(cap.payload.Body(), &got))
	assert.EqualValues(t, 8192, got["max_tokens"], "transform must reach the interceptor")
}

func TestBridgePolicy_ClassificationsRecorded(t *testing.T) {
	t.Parallel()

	classify, err := policy.NewClassify("tier-classifier", `
annotations := {"tier": "gold"}
`)
	require.NoError(t, err)
	pipe, err := policy.NewPipeline(policy.PipelineConfig{Classify: []*policy.Classify{classify}})
	require.NoError(t, err)
	bridge, rec, _ := newPolicyBridge(t, pipe)

	resp := policyRequest(t, bridge, `{"model":"gpt-4o"}`)
	require.Equal(t, http.StatusOK, resp.Code)

	interceptions := rec.RecordedInterceptions()
	require.Len(t, interceptions, 1)
	classifications, ok := interceptions[0].Metadata["classifications"].(map[string]any)
	require.True(t, ok, "classifications must be recorded under Metadata")
	assert.Equal(t, "gold", classifications["tier"])
	// Existing actor metadata is preserved alongside classifications.
	assert.Equal(t, "platform", interceptions[0].Metadata["team"])
}
