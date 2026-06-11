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
	// PreTool is the per-tool-call pipeline (classify + decide only). It is
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
// "absence = pass-through" posture.
type policyHooks struct {
	byProvider map[string]ProviderPipelines
}

// RequestBridgeOption configures optional [RequestBridge] behavior.
type RequestBridgeOption func(*requestBridgeOptions)

type requestBridgeOptions struct {
	hooks policyHooks
}

// WithPolicyHooks attaches per-provider policy pipelines keyed by provider name.
// Providers absent from the map run no policies.
func WithPolicyHooks(byProvider map[string]ProviderPipelines) RequestBridgeOption {
	return func(o *requestBridgeOptions) {
		o.hooks = policyHooks{byProvider: byProvider}
	}
}

// apply runs the pre-auth then pre-req pipelines for a request. It returns the
// (possibly rewritten) payload and the accumulated classification annotations
// to record. When it returns blocked=true it has already written the 403
// response and the caller must stop.
//
// Classification annotations from pre-auth are threaded into the pre-req
// envelope, so a later policy can read an earlier classifier's output.
func (h policyHooks) apply(w http.ResponseWriter, r *http.Request, payload intercept.Payload, actor *aibcontext.Actor, provider string, m *metrics.Metrics, logger slog.Logger) (_ intercept.Payload, annotations, modifications map[string]any, blocked bool) {
	ctx := r.Context()
	model := payload.Model()

	// Absence = pass-through: a provider with no configured pipelines runs no
	// policies.
	ph := h.byProvider[provider]

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
	// pre-req policy pipeline so their annotations are visible to it, an
	// enforcing guardrail's body rewrite is applied before any policy sees the
	// body, and an enforcing block short-circuits with HTTP 400.
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
				slog.F("hook", hookPreReq), slog.F("guardrail", ge.Name), slog.Error(ge.Err))
		}
		if gres.Block {
			msg := blockedByGuardrailMessage
			if gres.BlockedBy != "" {
				msg = fmt.Sprintf("request blocked by guardrail %q", gres.BlockedBy)
				if gres.Reason != "" {
					msg += ": " + gres.Reason
				}
			}
			logger.Debug(ctx, "request blocked by guardrail",
				slog.F("guardrail", gres.BlockedBy), slog.F("reason", gres.Reason))
			http.Error(w, msg, http.StatusBadRequest)
			return payload, nil, nil, true
		}
		if gres.Body != nil {
			payload = payload.WithBody(gres.Body)
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
// capturing the (post-pre-req) request body, identity, and accumulated
// annotations. It returns nil when the provider has no pre-tool pipeline, which
// makes tool gating a no-op for the request.
func (h policyHooks) toolGate(provider string, headers map[string]any, method, path string, body []byte, actor *aibcontext.Actor, annotations map[string]any, m *metrics.Metrics, logger slog.Logger) intercept.ToolGate {
	ph := h.byProvider[provider]
	if ph.PreTool == nil {
		return nil
	}
	return &policyToolGate{
		pipe:        ph.PreTool,
		failOpen:    ph.PreToolFailOpen,
		version:     ph.Version,
		provider:    provider,
		headers:     headers,
		method:      method,
		path:        path,
		body:        body,
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
	headers     map[string]any
	method      string
	path        string
	body        []byte
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
		ToolCall: policy.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
			Index:     call.Index,
		},
		Headers:  g.headers,
		Method:   g.method,
		Path:     g.path,
		Request:  g.body,
		Identity: g.identity,
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

// metadataWithPolicyResults returns md with classify annotations attached under
// the "classifications" key and policy modifications under the "modifications"
// key, without mutating the caller's map. md is returned unchanged when both are
// empty.
func metadataWithPolicyResults(md recorder.Metadata, classifications, modifications map[string]any) recorder.Metadata {
	if len(classifications) == 0 && len(modifications) == 0 {
		return md
	}
	out := maps.Clone(md)
	if out == nil {
		out = recorder.Metadata{}
	}
	if len(classifications) > 0 {
		out["classifications"] = classifications
	}
	if len(modifications) > 0 {
		out["modifications"] = modifications
	}
	return out
}
