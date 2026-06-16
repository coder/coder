package aibridge

import (
	"context"
	"fmt"
	"maps"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/guardrail"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/policy"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// blockedByPolicyMessage is written to clients whose request a policy denied.
const blockedByPolicyMessage = "request blocked by policy"

// blockedByGuardrailMessage is written to clients whose request a guardrail
// rejected. Both policy and guardrail blocks return HTTP 400.
const blockedByGuardrailMessage = "request blocked by guardrail"

// Hook names used as metric labels.
const (
	hookPreAuth = "pre-auth"
	hookPreReq  = "pre-req"
	hookPreTool = "pre-tool"
)

// HeaderAIGatewayPipelineVersion is an owner-only request header naming a
// specific pipeline version by its logical version number (e.g. "3" or the UI's
// "v3" label, both accepted) to evaluate against the request instead of the
// active version. It powers version-targeted evaluation (§10.9): an owner
// rehearses an unpromoted posture against real traffic before promoting it. A
// non-owner
// sending the header is rejected with HTTP 403; a non-numeric, unknown, or
// foreign version is rejected with HTTP 400. Absent, the active version
// evaluates (the unchanged hot path). The header is stripped before the request
// is forwarded upstream so it never reaches the provider.
const HeaderAIGatewayPipelineVersion = "X-Coder-AI-Gateway-Pipeline-Version"

// ErrPipelineVersionNotFound is returned by a [PipelineResolver] when the
// requested pipeline version does not exist or does not belong to the request's
// provider. The host maps it to a client 4xx.
var ErrPipelineVersionNotFound = xerrors.New("pipeline version not found")

// PipelineResolver resolves the compiled pipeline snapshot for a specific
// pipeline version. It is implemented by the host (coderd/aibridged), which has
// database access; the bridge library itself is database-less. Implementations
// should cache by version id since versions are immutable.
type PipelineResolver interface {
	// ResolvePipelineVersion returns the compiled pipelines for the given logical
	// version number (the X-Coder-AI-Gateway-Pipeline-Version header value),
	// which must belong to the named provider's pipeline. It returns
	// [ErrPipelineVersionNotFound] when the version is non-numeric, unknown, or
	// foreign.
	ResolvePipelineVersion(ctx context.Context, provider, version string) (ProviderPipelines, error)
}

// ProviderPipelines holds the compiled per-hook policy pipelines for a single
// provider. A nil pipeline is a no-op for that hook.
//
// Pipelines are compiled once (compile-once, eval-many); only evaluation runs
// per request.
type ProviderPipelines struct {
	PreAuth *policy.Pipeline
	PreReq  *policy.Pipeline
	// PreReqGuardrails is the networked head-of-hook guardrail stage for the
	// pre-req hook. It runs before PreReq: it may block the request (HTTP 400),
	// rewrite the body (masking), or attach annotations that the PreReq policy
	// pipeline can read. A nil or empty stage is a no-op.
	PreReqGuardrails *guardrail.Stage
	// PreTool is the per-tool-call pipeline (annotate + decide only). It is
	// evaluated once per assembled, client-bound tool call before the call is
	// released to the client. A nil pipeline is a no-op for that hook.
	PreTool *policy.Pipeline
	// PreToolFailOpen is the aggregate fail mode of the PreTool pipeline: true
	// only when every decide member is fail-open. It governs the interceptor's
	// behavior on an over-cap or incomplete held block that cannot be evaluated.
	PreToolFailOpen bool
	// Version is the active pipeline version that produced these pipelines. It
	// is recorded on policy decisions so an audit can reconstruct exactly which
	// version evaluated a past request.
	Version int32
}

// policyHooks holds the compiled pipelines keyed by provider name. A provider
// with no entry runs no policies (pass-through), matching the design's
// "absence = pass-through" posture. resolver, when set, supplies the compiled
// pipelines for an owner-selected pipeline version (§10.9).
type policyHooks struct {
	byProvider map[string]ProviderPipelines
	resolver   PipelineResolver
}

// RequestBridgeOption configures optional [RequestBridge] behavior.
type RequestBridgeOption func(*requestBridgeOptions)

type requestBridgeOptions struct {
	byProvider map[string]ProviderPipelines
	resolver   PipelineResolver
}

// WithPolicyHooks attaches per-provider policy pipelines keyed by provider name.
// Providers absent from the map run no policies.
func WithPolicyHooks(byProvider map[string]ProviderPipelines) RequestBridgeOption {
	return func(o *requestBridgeOptions) {
		o.byProvider = byProvider
	}
}

// WithPipelineResolver attaches a resolver used to compile a specific pipeline
// version on demand for owner-only version-targeted evaluation (§10.9). Absent,
// the version header is rejected with HTTP 400 (feature unavailable).
func WithPipelineResolver(r PipelineResolver) RequestBridgeOption {
	return func(o *requestBridgeOptions) {
		o.resolver = r
	}
}

// apply runs the pre-auth then pre-req pipelines for a request. It returns the
// (possibly rewritten) payload and the accumulated annotations to record. When
// it returns blocked=true it has already written the 403 response and the
// caller must stop.
//
// Annotations from pre-auth are threaded into the pre-req envelope, so a later
// policy can read an earlier annotate stage's output.
func (h policyHooks) apply(w http.ResponseWriter, r *http.Request, payload intercept.Payload, actor *aibcontext.Actor, ph ProviderPipelines, provider string, m *metrics.Metrics, logger slog.Logger) (_ intercept.Payload, annotations, modifications map[string]any, blocked bool) {
	ctx := r.Context()
	model := payload.Model()

	if ph.PreAuth != nil {
		in, err := policy.PreAuthEnvelope{
			Headers:    headerMap(r.Header, remoteIP(r)),
			Credential: credentialMap(r),
		}.Build()
		if err != nil {
			p, a, b := h.fail(w, ctx, hookPreAuth, "build pre-auth input", err, logger)
			return p, a, nil, b
		}
		res, ok := h.evaluate(w, ctx, ph.PreAuth, in, hookPreAuth, provider, model, ph.Version, m, logger)
		if !ok {
			return payload, nil, nil, true
		}
		annotations = res.Annotations
		modifications = res.Modifications
	}

	// Guardrails are the networked head-of-hook stage: they run before the
	// pre-req policy pipeline so their annotations are visible to it, a
	// guardrail's body rewrite is applied before any policy sees the body, and a
	// guardrail block short-circuits with HTTP 400.
	if g := ph.PreReqGuardrails; g != nil && !g.Empty() {
		gres, err := g.Run(ctx, payload.Body(), payload.Model())
		if err != nil {
			// An internal error (e.g. applying a body edit) fails closed.
			logger.Warn(ctx, "evaluate guardrails", slog.F("hook", hookPreReq), slog.Error(err))
			http.Error(w, blockedByGuardrailMessage, http.StatusBadRequest)
			return payload, nil, nil, true
		}
		// Surface the underlying cause of any guardrail failure (the generic
		// client message hides it). This is the only place a vendor outage,
		// wrong endpoint, or timeout is visible.
		for _, ge := range gres.Errors {
			logger.Warn(ctx, "guardrail evaluation failed",
				slog.F("hook", hookPreReq), slog.F("guardrail", ge.Stage), slog.Error(ge.Err))
		}
		if gres.Verdict.Blocks() {
			msg := blockedByGuardrailMessage
			// BlockedBy is set only for a deliberate guardrail block; a
			// synthesized failure block stays anonymous to the client.
			if gres.BlockedBy != "" {
				msg = fmt.Sprintf("request blocked by guardrail %q", gres.BlockedBy)
				if gres.Message != "" {
					msg += ": " + gres.Message
				}
			}
			logger.Debug(ctx, "request blocked by guardrail",
				slog.F("guardrail", gres.BlockedBy), slog.F("reason", gres.Message))
			http.Error(w, msg, http.StatusBadRequest)
			return payload, nil, nil, true
		}
		if gres.RequestBody != nil {
			payload = payload.WithBody(gres.RequestBody)
		}
		annotations = mergeAnnotations(annotations, gres.Annotations)
	}

	if ph.PreReq != nil {
		in, err := policy.PreReqEnvelope{
			Headers:  headerMap(r.Header, remoteIP(r)),
			Method:   r.Method,
			Path:     r.URL.Path,
			Request:  payload.Body(),
			Identity: identityFromActor(actor),
		}.Build()
		if err != nil {
			p, a, b := h.fail(w, ctx, hookPreReq, "build pre-req input", err, logger)
			return p, a, nil, b
		}
		if len(annotations) > 0 {
			in, err = in.WithAnnotations(annotations)
			if err != nil {
				p, a, b := h.fail(w, ctx, hookPreReq, "thread annotations", err, logger)
				return p, a, nil, b
			}
		}
		res, ok := h.evaluate(w, ctx, ph.PreReq, in, hookPreReq, provider, model, ph.Version, m, logger)
		if !ok {
			return payload, nil, nil, true
		}
		if res.RequestBody != nil {
			payload = payload.WithBody(res.RequestBody)
		}
		// Apply transform header overrides to the request before the interceptor
		// captures r.Header. Transport, auth, and hop-by-hop headers are
		// sanitized downstream by intercept.PrepareClientHeaders, so a policy
		// cannot inject credentials or corrupt framing this way.
		for k, v := range res.Headers {
			r.Header.Set(k, v)
		}
		annotations = mergeAnnotations(annotations, res.Annotations)
		modifications = mergeAnnotations(modifications, res.Modifications)
	}

	return payload, annotations, modifications, false
}

// toolGate builds an [intercept.ToolGate] for the provider's pre-tool pipeline,
// capturing the identity and accumulated annotations. The pre-tool envelope
// gates the individual tool call, so it does not carry the request body or
// headers. It returns nil when the provider has no pre-tool pipeline, which
// makes tool gating a no-op for the request.
func (h policyHooks) toolGate(ph ProviderPipelines, provider string, actor *aibcontext.Actor, annotations map[string]any, m *metrics.Metrics, logger slog.Logger) intercept.ToolGate {
	if ph.PreTool == nil {
		return nil
	}
	return &policyToolGate{
		pipe:        ph.PreTool,
		failOpen:    ph.PreToolFailOpen,
		version:     ph.Version,
		provider:    provider,
		identity:    identityFromActor(actor),
		annotations: annotations,
		metrics:     m,
		logger:      logger.With(slog.F("hook", hookPreTool), slog.F("pipeline_version", ph.Version)),
	}
}

// policyToolGate adapts a pre-tool [policy.Pipeline] to [intercept.ToolGate].
type policyToolGate struct {
	pipe        *policy.Pipeline
	failOpen    bool
	version     int32
	provider    string
	identity    policy.Identity
	annotations map[string]any
	metrics     *metrics.Metrics
	logger      slog.Logger
}

func (g *policyToolGate) FailOpen() bool { return g.failOpen }

func (g *policyToolGate) ObserveHold(seconds float64) {
	if g.metrics != nil {
		g.metrics.PolicyToolHoldDuration.WithLabelValues(g.provider).Observe(seconds)
	}
}

func (g *policyToolGate) EvaluateToolCall(ctx context.Context, call intercept.ToolCall) (intercept.ToolGateDecision, error) {
	in, err := policy.PreToolEnvelope{
		Identity: g.identity,
		ToolCall: policy.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
			Index:     call.Index,
		},
	}.Build()
	if err != nil {
		return intercept.ToolGateDecision{}, xerrors.Errorf("build pre-tool input: %w", err)
	}
	if len(g.annotations) > 0 {
		in, err = in.WithAnnotations(g.annotations)
		if err != nil {
			return intercept.ToolGateDecision{}, xerrors.Errorf("thread annotations: %w", err)
		}
	}
	res, err := g.pipe.Evaluate(ctx, in)
	if err != nil {
		return intercept.ToolGateDecision{}, err
	}
	if g.metrics != nil {
		g.metrics.PolicyToolVerdictCount.WithLabelValues(g.provider, call.Name, string(res.Verdict)).Inc()
	}
	if res.Verdict.Blocks() {
		g.logger.Debug(ctx, "tool call blocked by policy",
			slog.F("tool", call.Name), slog.F("policy", res.BlockedBy))
		reason := fmt.Sprintf("tool call %q blocked by policy %q", call.Name, res.BlockedBy)
		// An author-supplied message overrides the generic reason.
		if res.Message != "" {
			reason = res.Message
		}
		return intercept.ToolGateDecision{
			Block:     true,
			BlockedBy: res.BlockedBy,
			Reason:    reason,
		}, nil
	}
	return intercept.ToolGateDecision{}, nil
}

// evaluate runs a pipeline, records metrics, and blocks the response on a BLOCK
// verdict. ok is false when the request was blocked (response already written).
func (h policyHooks) evaluate(w http.ResponseWriter, ctx context.Context, pipe *policy.Pipeline, in policy.Input, hook, provider, model string, pipelineVersion int32, m *metrics.Metrics, logger slog.Logger) (policy.Result, bool) {
	// pipeline_version is recorded on every policy outcome so audits can
	// reconstruct which version evaluated a past request after a swap.
	logger = logger.With(slog.F("hook", hook), slog.F("pipeline_version", pipelineVersion))
	start := time.Now()
	res, err := pipe.Evaluate(ctx, in)
	if m != nil {
		m.PolicyEvalDuration.WithLabelValues(provider, hook).Observe(time.Since(start).Seconds())
	}
	if err != nil {
		// A pipeline only returns an error on an internal failure (not a
		// policy verdict); fail closed.
		logger.Warn(ctx, "evaluate policies", slog.Error(err))
		http.Error(w, blockedByPolicyMessage, http.StatusBadRequest)
		return policy.Result{}, false
	}
	if m != nil {
		m.PolicyVerdictCount.WithLabelValues(provider, model, hook, string(res.Verdict)).Inc()
	}
	// Surface fail-open stage failures (normalized to LOG). The generic client
	// message hides them, so this is the only place a fail-open eval error or
	// timeout is visible in the log stream.
	for _, se := range res.Errors {
		logger.Warn(ctx, "policy stage failed open",
			slog.F("policy", se.Stage), slog.Error(se.Err))
	}
	if res.Verdict.Blocks() {
		msg := blockedByPolicyMessage
		if res.BlockedBy != "" {
			msg = fmt.Sprintf("request blocked by policy %q", res.BlockedBy)
		}
		// An author-supplied message overrides the generic one.
		if res.Message != "" {
			msg = res.Message
		}
		logger.Debug(ctx, "request blocked by policy", slog.F("policy", res.BlockedBy))
		http.Error(w, msg, http.StatusBadRequest)
		return policy.Result{}, false
	}
	for name, mod := range res.Modifications {
		logger.Debug(ctx, "policy modified request",
			slog.F("policy", name), slog.F("modification", mod))
	}
	return res, true
}

func (policyHooks) fail(w http.ResponseWriter, ctx context.Context, hook, msg string, err error, logger slog.Logger) (intercept.Payload, map[string]any, bool) {
	logger.Warn(ctx, msg, slog.F("hook", hook), slog.Error(err))
	http.Error(w, blockedByPolicyMessage, http.StatusBadRequest)
	return intercept.Payload{}, nil, true
}

// remoteIP extracts the host part of r.RemoteAddr, returning an empty string
// on parse failure. It does not consult forwarding headers; that is left to
// policy authors who can read input.headers["x-forwarded-for"] directly.
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	return host
}

// headerMap converts HTTP headers to a flat map of lowercase name→first value.
// remoteAddr is the parsed TCP remote address (from r.RemoteAddr); it is added
// as "x-remote-addr" so policies can read it from input.headers alongside real
// request headers without needing a separate envelope field.
func headerMap(h http.Header, remoteAddr string) map[string]any {
	m := make(map[string]any, len(h)+1)
	for k, v := range h {
		if len(v) > 0 {
			m[strings.ToLower(k)] = v[0]
		}
	}
	m["x-remote-addr"] = remoteAddr
	return m
}

func credentialMap(r *http.Request) map[string]any {
	return map[string]any{
		"authorization": r.Header.Get("Authorization"),
		"x_api_key":     r.Header.Get("X-Api-Key"),
	}
}

// identityFromActor maps the request actor to the typed policy identity. Only
// id and username are available today; groups and roles are reserved (empty)
// until the IsAuthorized RPC carries them (see aibridgedserver.IsAuthorized).
// This is intentionally separate from actor.Metadata, which is forwarded
// upstream and must stay non-sensitive.
func identityFromActor(actor *aibcontext.Actor) policy.Identity {
	if actor == nil {
		return policy.Identity{}
	}
	username, _ := actor.Metadata["Username"].(string)
	return policy.Identity{ID: actor.ID, Username: username}
}

// actorIsOwner reports whether the actor holds the site-wide owner role, read
// from the actor metadata set by the host (coderd/aibridged) from the
// IsAuthorized RPC. It gates owner-only version-targeted evaluation (§10.9).
func actorIsOwner(actor *aibcontext.Actor) bool {
	if actor == nil {
		return false
	}
	owner, _ := actor.Metadata["IsOwner"].(bool)
	return owner
}

func mergeAnnotations(dst, src map[string]any) map[string]any {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]any, len(src))
	}
	maps.Copy(dst, src)
	return dst
}

// metadataWithPolicyResults returns md with annotate/guardrail annotations
// attached under the "annotations" key and policy modifications under the
// "modifications" key, without mutating the caller's map. md is returned
// unchanged when both are empty.
func metadataWithPolicyResults(md recorder.Metadata, annotations, modifications map[string]any) recorder.Metadata {
	if len(annotations) == 0 && len(modifications) == 0 {
		return md
	}
	out := maps.Clone(md)
	if out == nil {
		out = recorder.Metadata{}
	}
	if len(annotations) > 0 {
		out["annotations"] = annotations
	}
	if len(modifications) > 0 {
		out["modifications"] = modifications
	}
	return out
}
