package coderd

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	experimentrules "github.com/coder/coder/v2/coderd/experiments"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get enabled experiments
// @ID get-enabled-experiments
// @Security CoderSessionToken
// @Produce json
// @Tags General
// @Success 200 {array} codersdk.Experiment
// @Router /api/v2/experiments [get]
func (api *API) handleExperimentsGet(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	// The result is personalized by the caller's rules, so it is served
	// with no-store caching (see the route registration).
	httpapi.Write(ctx, rw, http.StatusOK, api.ExperimentEvaluator.EnabledExperiments(ctx, apiKey.UserID))
}

// @Summary Get safe experiments
// @ID get-safe-experiments
// @Security CoderSessionToken
// @Produce json
// @Tags General
// @Success 200 {array} codersdk.Experiment
// @Router /api/v2/experiments/available [get]
func handleExperimentsAvailable(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AvailableExperiments{
		Safe: codersdk.ExperimentsSafe,
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary List experiment rules
// @ID list-experiment-rules
// @Security CoderSessionToken
// @Produce json
// @Tags General
// @Success 200 {array} codersdk.ExperimentRuleEntry
// @Router /api/experimental/experiments/rules [get]
// @x-apidocgen {"skip": true}
func (api *API) experimentRules(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	rules, err := experimentrules.ReadRules(ctx, api.Database)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	entries := make([]codersdk.ExperimentRuleEntry, 0, len(codersdk.ExperimentsUserScoped)+len(rules))
	for _, ex := range codersdk.ExperimentsUserScoped {
		entry := codersdk.ExperimentRuleEntry{
			Experiment:    ex,
			StaticDefault: api.Experiments.Enabled(ex),
		}
		if rule, ok := rules[ex]; ok {
			entry.Rule = ptr.Ref(convertExperimentRule(rule))
		}
		entries = append(entries, entry)
	}
	ignored := make([]codersdk.Experiment, 0, len(rules))
	for ex := range rules {
		if !experimentrules.IsUserScoped(ex) {
			ignored = append(ignored, ex)
		}
	}
	slices.Sort(ignored)
	for _, ex := range ignored {
		entries = append(entries, codersdk.ExperimentRuleEntry{
			Experiment:    ex,
			StaticDefault: api.Experiments.Enabled(ex),
			Rule:          ptr.Ref(convertExperimentRule(rules[ex])),
			Ignored:       true,
		})
	}
	httpapi.Write(ctx, rw, http.StatusOK, entries)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Update experiment rule
// @ID update-experiment-rule
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags General
// @Param experiment path string true "Experiment name"
// @Param request body codersdk.PutExperimentRuleRequest true "Experiment rule"
// @Success 200 {object} codersdk.ExperimentRule
// @Router /api/experimental/experiments/rules/{experiment} [put]
// @x-apidocgen {"skip": true}
func (api *API) putExperimentRule(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	// Authorize before reading the body, so that callers without access
	// never reach the CEL compiler. Like other deployment settings, a
	// denied request is not audited.
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig) ||
		!api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	ex := codersdk.Experiment(chi.URLParam(r, "experiment"))
	var req codersdk.PutExperimentRuleRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if !experimentrules.IsUserScoped(ex) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Experiment %q does not accept runtime rules.", ex),
		})
		return
	}
	if req.ExpectedRevision < 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Expected revision must not be negative.",
			Validations: []codersdk.ValidationError{{
				Field:  "expected_revision",
				Detail: fmt.Sprintf("Got %d.", req.ExpectedRevision),
			}},
		})
		return
	}
	rule := experimentrules.Rule{
		Mode:      experimentrules.Mode(req.Mode),
		Condition: req.Condition,
	}
	if err := experimentrules.ValidateRule(ex, rule); err != nil {
		// The error carries full CEL diagnostics. Only the author of the
		// condition receives them; they are never logged or audited.
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid experiment rule.",
			Detail:  err.Error(),
		})
		return
	}

	// From here on, failures are audited with their status and an empty
	// diff. The audit entry is exported after the handler returns, outside
	// the write transaction.
	aReq, commitAudit := audit.InitRequestWithCancel[database.ExperimentRule](rw, &audit.RequestParams{
		Audit:   *api.Auditor.Load(),
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit(true)
	aReq.New = experimentrules.AuditRecord(ex, experimentrules.Rule{})

	oldRule, newRule, changed, err := experimentrules.WriteRule(ctx, api.Database, apiKey.UserID, ex, rule, req.ExpectedRevision)
	if err != nil {
		var conflict *experimentrules.RevisionConflictError
		if errors.As(err, &conflict) {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: fmt.Sprintf("The rule for experiment %q was changed by someone else.", ex),
				Detail:  fmt.Sprintf("The current revision is %d, but the request expected revision %d. Read the current rule and retry.", conflict.Current, conflict.Expected),
			})
			return
		}
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	if !changed {
		// An identical rule changes nothing, so nothing is audited.
		commitAudit(false)
		httpapi.Write(ctx, rw, http.StatusOK, convertExperimentRule(newRule))
		return
	}
	aReq.Old = experimentrules.AuditRecord(ex, oldRule)
	aReq.New = experimentrules.AuditRecord(ex, newRule)
	httpapi.Write(ctx, rw, http.StatusOK, convertExperimentRule(newRule))
}

func convertExperimentRule(rule experimentrules.Rule) codersdk.ExperimentRule {
	return codersdk.ExperimentRule{
		Mode:      codersdk.ExperimentRuleMode(rule.Mode),
		Condition: rule.Condition,
		Revision:  rule.Revision,
		UpdatedBy: rule.UpdatedBy,
		UpdatedAt: rule.UpdatedAt,
	}
}
