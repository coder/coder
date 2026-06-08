package aibridge

import (
	"context"
	"fmt"
	"maps"
	"net"
	"net/http"
	"strings"
	"time"

	"cdr.dev/slog/v3"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/policy"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// blockedByPolicyMessage is written to clients whose request a policy denied.
const blockedByPolicyMessage = "request blocked by policy"

// Hook names used as metric labels.
const (
	hookPreAuth = "pre-auth"
	hookPreReq  = "pre-req"
)

// ProviderPipelines holds the compiled per-hook policy pipelines for a single
// provider. A nil pipeline is a no-op for that hook.
//
// Pipelines are compiled once (compile-once, eval-many); only evaluation runs
// per request.
type ProviderPipelines struct {
	PreAuth *policy.Pipeline
	PreReq  *policy.Pipeline
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
		in, err := policy.BuildAuthInput(headerMap(r.Header, remoteIP(r)), credentialMap(r))
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

	if ph.PreReq != nil {
		in, err := policy.BuildInput(payload.Body(), identityMap(actor))
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
		annotations = mergeAnnotations(annotations, res.Annotations)
		modifications = mergeAnnotations(modifications, res.Modifications)
	}

	return payload, annotations, modifications, false
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
		http.Error(w, blockedByPolicyMessage, http.StatusForbidden)
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
		logger.Debug(ctx, "request blocked by policy", slog.F("policy", res.BlockedBy))
		http.Error(w, msg, http.StatusForbidden)
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
	http.Error(w, blockedByPolicyMessage, http.StatusForbidden)
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

func identityMap(actor *aibcontext.Actor) map[string]any {
	if actor == nil {
		return nil
	}
	return map[string]any{
		"id":       actor.ID,
		"metadata": map[string]any(actor.Metadata),
	}
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
