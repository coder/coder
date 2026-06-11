package aibridge_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/policy"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// recorderMetadataWithOwner builds actor metadata carrying the owner flag the
// host (coderd/aibridged) sets from the IsAuthorized RPC.
func recorderMetadataWithOwner(isOwner bool) recorder.Metadata {
	return recorder.Metadata{"Username": "user-1", "IsOwner": isOwner}
}

// fakeResolver implements aibridge.PipelineResolver for one known
// (provider, version) pair, returning a fixed pipeline. Any other input yields
// ErrPipelineVersionNotFound.
type fakeResolver struct {
	calls         int
	knownProvider string
	knownVersion  string
	pipe          *policy.Pipeline
}

func (r *fakeResolver) ResolvePipelineVersion(_ context.Context, provider, version string) (aibridge.ProviderPipelines, error) {
	r.calls++
	if provider != r.knownProvider || version != r.knownVersion {
		return aibridge.ProviderPipelines{}, aibridge.ErrPipelineVersionNotFound
	}
	return aibridge.ProviderPipelines{PreReq: r.pipe, Version: 99}, nil
}

// newVersionTargetedBridge builds a bridge with an active pre-req pipeline and a
// resolver for version-targeted evaluation. resolver may be nil to exercise the
// "feature unavailable" path.
func newVersionTargetedBridge(t *testing.T, active *policy.Pipeline, resolver aibridge.PipelineResolver) (http.Handler, *capture) {
	t.Helper()
	cap := &capture{}
	prov := &testutil.MockProvider{
		NameStr: "mock",
		Bridged: []string{"/v1/messages"},
		InterceptorFunc: func(_ http.ResponseWriter, r *http.Request, payload intercept.Payload, _ trace.Tracer) (intercept.Interceptor, error) {
			cap.called = true
			cap.payload = payload
			cap.request = r
			return &fakeInterceptor{id: uuid.New(), model: payload.Model()}, nil
		},
	}
	opts := []aibridge.RequestBridgeOption{
		aibridge.WithPolicyHooks(map[string]aibridge.ProviderPipelines{
			"mock": {PreReq: active},
		}),
	}
	if resolver != nil {
		opts = append(opts, aibridge.WithPipelineResolver(resolver))
	}
	bridge, err := aibridge.NewRequestBridge(
		t.Context(), []provider.Provider{prov}, &testutil.MockRecorder{}, nil,
		slogtest.Make(t, nil), nil, bridgeTestTracer, opts...,
	)
	require.NoError(t, err)
	return bridge, cap
}

// versionTargetedRequest sends a request through the bridge with the optional
// version header and an actor whose owner-ness is controlled by isOwner.
func versionTargetedRequest(t *testing.T, bridge http.Handler, body, versionHeader string, isOwner bool) *httptest.ResponseRecorder {
	t.Helper()
	ctx := aibridge.AsActor(t.Context(), "user-1", recorderMetadataWithOwner(isOwner))
	req := httptest.NewRequest(http.MethodPost, "/mock/v1/messages", strings.NewReader(body)).WithContext(ctx)
	if versionHeader != "" {
		req.Header.Set(aibridge.HeaderAIGatewayPipelineVersion, versionHeader)
	}
	resp := httptest.NewRecorder()
	bridge.ServeHTTP(resp, req)
	return resp
}

func allowPipeline(t *testing.T) *policy.Pipeline {
	t.Helper()
	pipe, err := policy.NewPipeline(policy.PipelineConfig{
		Decide: []*policy.Decide{mustDecide(t, "allow-all", `default verdict := "ALLOW"`)},
	})
	require.NoError(t, err)
	return pipe
}

func blockPipeline(t *testing.T) *policy.Pipeline {
	t.Helper()
	pipe, err := policy.NewPipeline(policy.PipelineConfig{
		Decide: []*policy.Decide{mustDecide(t, "staged-blocker", `default verdict := "BLOCK"`)},
	})
	require.NoError(t, err)
	return pipe
}

func TestVersionTargeted_NoHeaderUsesActive(t *testing.T) {
	t.Parallel()

	resolver := &fakeResolver{knownProvider: "mock", knownVersion: "2", pipe: blockPipeline(t)}
	bridge, cap := newVersionTargetedBridge(t, allowPipeline(t), resolver)

	resp := versionTargetedRequest(t, bridge, `{"model":"m"}`, "", true)
	assert.Equal(t, http.StatusOK, resp.Code)
	assert.True(t, cap.called, "active (ALLOW) pipeline must run when no header is set")
	assert.Zero(t, resolver.calls, "resolver must not be consulted without the header")
}

func TestVersionTargeted_OwnerHeaderUsesResolvedVersion(t *testing.T) {
	t.Parallel()

	version := "2"
	resolver := &fakeResolver{knownProvider: "mock", knownVersion: version, pipe: blockPipeline(t)}
	// Active pipeline allows; the staged version blocks. With the header, the
	// staged (BLOCK) version must evaluate.
	bridge, cap := newVersionTargetedBridge(t, allowPipeline(t), resolver)

	resp := versionTargetedRequest(t, bridge, `{"model":"m"}`, version, true)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), `request blocked by policy "staged-blocker"`)
	assert.False(t, cap.called, "interceptor must not run when the staged version blocks")
	assert.Equal(t, 1, resolver.calls)
}

func TestVersionTargeted_NonOwnerForbidden(t *testing.T) {
	t.Parallel()

	version := "2"
	resolver := &fakeResolver{knownProvider: "mock", knownVersion: version, pipe: blockPipeline(t)}
	bridge, cap := newVersionTargetedBridge(t, allowPipeline(t), resolver)

	resp := versionTargetedRequest(t, bridge, `{"model":"m"}`, version, false)
	assert.Equal(t, http.StatusForbidden, resp.Code)
	assert.False(t, cap.called)
	assert.Zero(t, resolver.calls, "a non-owner must be rejected before the resolver is consulted")
}

func TestVersionTargeted_UnknownVersionRejected(t *testing.T) {
	t.Parallel()

	resolver := &fakeResolver{knownProvider: "mock", knownVersion: "2", pipe: blockPipeline(t)}
	bridge, cap := newVersionTargetedBridge(t, allowPipeline(t), resolver)

	resp := versionTargetedRequest(t, bridge, `{"model":"m"}`, "999", true)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "unknown pipeline version")
	assert.False(t, cap.called)
}

func TestVersionTargeted_NoResolverUnavailable(t *testing.T) {
	t.Parallel()

	bridge, cap := newVersionTargetedBridge(t, allowPipeline(t), nil)

	resp := versionTargetedRequest(t, bridge, `{"model":"m"}`, "2", true)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "unavailable")
	assert.False(t, cap.called)
}

func TestVersionTargeted_HeaderStrippedFromUpstream(t *testing.T) {
	t.Parallel()

	version := "2"
	// Resolve to an ALLOW pipeline so the request proceeds to the interceptor,
	// where we can inspect the forwarded headers.
	resolver := &fakeResolver{knownProvider: "mock", knownVersion: version, pipe: allowPipeline(t)}
	bridge, cap := newVersionTargetedBridge(t, allowPipeline(t), resolver)

	resp := versionTargetedRequest(t, bridge, `{"model":"m"}`, version, true)
	require.Equal(t, http.StatusOK, resp.Code)
	require.True(t, cap.called)
	assert.Empty(t, cap.request.Header.Get(aibridge.HeaderAIGatewayPipelineVersion),
		"the version header must be stripped before the request is forwarded")
}
