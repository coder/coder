package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

// ExperimentRuleMode selects how a runtime experiment rule decides an
// experiment.
type ExperimentRuleMode string

const (
	// ExperimentRuleModeInherit uses the startup --experiments default.
	ExperimentRuleModeInherit ExperimentRuleMode = "inherit"
	// ExperimentRuleModeOn enables the experiment for every user.
	ExperimentRuleModeOn ExperimentRuleMode = "on"
	// ExperimentRuleModeOff disables the experiment for every user, even
	// when it is enabled at startup.
	ExperimentRuleModeOff ExperimentRuleMode = "off"
	// ExperimentRuleModeCondition enables the experiment for users whose
	// CEL condition is true.
	ExperimentRuleModeCondition ExperimentRuleMode = "condition"
)

// ExperimentRule is the stored runtime rule of one experiment.
type ExperimentRule struct {
	// Mode is empty when the stored rule is malformed. A malformed rule
	// decides off until it is replaced.
	Mode ExperimentRuleMode `json:"mode" enums:"inherit,on,off,condition"`
	// Condition is the CEL expression of a condition rule.
	Condition string `json:"condition,omitempty"`
	// Revision increases on every change. Zero means never configured.
	Revision  int64     `json:"revision"`
	UpdatedBy uuid.UUID `json:"updated_by" format:"uuid"`
	UpdatedAt time.Time `json:"updated_at" format:"date-time"`
}

// ExperimentRuleEntry describes the runtime rule state of one experiment.
type ExperimentRuleEntry struct {
	Experiment Experiment `json:"experiment"`
	// StaticDefault reports whether the experiment is in the startup
	// --experiments list of the replica that answered.
	StaticDefault bool `json:"static_default"`
	// Rule is null when no rule was ever stored.
	Rule *ExperimentRule `json:"rule"`
	// Ignored is true for a stored rule of an experiment that does not
	// accept runtime rules. Such a rule has no effect.
	Ignored bool `json:"ignored"`
}

// PutExperimentRuleRequest replaces the runtime rule of one experiment.
type PutExperimentRuleRequest struct {
	Mode ExperimentRuleMode `json:"mode" enums:"inherit,on,off,condition"`
	// Condition is required for the condition mode and must be empty
	// otherwise.
	Condition string `json:"condition,omitempty"`
	// ExpectedRevision must equal the current revision of the stored rule,
	// or zero when no rule is stored. A different revision fails with 409
	// Conflict.
	ExpectedRevision int64 `json:"expected_revision"`
}

// ExperimentRules returns the runtime rule state of every experiment that
// accepts runtime rules, followed by ignored stored rules.
func (c *ExperimentalClient) ExperimentRules(ctx context.Context) ([]ExperimentRuleEntry, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/experiments/rules", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var entries []ExperimentRuleEntry
	return entries, ReadBodyAsJSON(res, &entries)
}

// PutExperimentRule replaces the runtime rule of ex and returns the stored
// rule. A stale expected revision returns an *Error with status 409.
func (c *ExperimentalClient) PutExperimentRule(ctx context.Context, ex Experiment, req PutExperimentRuleRequest) (ExperimentRule, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/experimental/experiments/rules/%s", url.PathEscape(string(ex))), req)
	if err != nil {
		return ExperimentRule{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ExperimentRule{}, ReadBodyAsError(res)
	}
	var rule ExperimentRule
	return rule, ReadBodyAsJSON(res, &rule)
}
