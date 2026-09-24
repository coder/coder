// Package experiments decides whether an experiment is enabled for a
// subject user. Each user-scoped experiment can have one stored rule that
// inherits the startup default, forces it on or off, or targets users with
// a CEL condition. Every error decides off (fail closed). Decisions gate
// features; they are not an authorization boundary.
package experiments

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// Mode selects how a rule decides an experiment.
type Mode string

const (
	// ModeInherit uses the startup --experiments default.
	ModeInherit Mode = "inherit"
	// ModeOn enables the experiment for every user.
	ModeOn Mode = "on"
	// ModeOff disables the experiment for every user, even when it is
	// enabled at startup.
	ModeOff Mode = "off"
	// ModeCondition enables the experiment for users whose condition is
	// true.
	ModeCondition Mode = "condition"
)

// Rule is the decoded form of one stored experiment rule.
type Rule struct {
	Mode Mode `json:"mode"`
	// Condition is a CEL expression over `user`. It is set only when Mode
	// is ModeCondition.
	Condition string `json:"condition,omitempty"`
	// Revision increases on every write. Zero means never configured.
	Revision  int64     `json:"revision"`
	UpdatedBy uuid.UUID `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StoredRule is the raw stored value of one experiment rule. It stays
// undecoded so that the Evaluator decides how undecodable values and
// unknown modes behave: they decide off.
type StoredRule struct {
	Value json.RawMessage
}

// User holds the attributes a condition can read as `user`. Only string
// and []string fields are allowed. Fields carry `cel` tags and no `json`
// tags so that CEL exposes exactly these names.
type User struct {
	ID       string `cel:"id"`
	Username string `cel:"username"`
	// Email is compared as stored (case-sensitive).
	Email string `cel:"email"`
	// Roles are the explicit site roles. The implied member role and
	// organization roles are not included.
	Roles []string `cel:"roles"`
	// Organizations are the names of organizations the user belongs to.
	Organizations []string `cel:"organizations"`
	// Groups are "<org>/<group>" names, including "<org>/Everyone".
	Groups []string `cel:"groups"`
}

// IsUserScoped reports whether ex accepts runtime rules.
func IsUserScoped(ex codersdk.Experiment) bool {
	return slices.Contains(codersdk.ExperimentsUserScoped, ex)
}

// ValidateRule checks that rule can be stored for ex. The returned error
// can contain full CEL diagnostics, which may quote the condition. Return
// it only to the rule author; never log it.
func ValidateRule(ex codersdk.Experiment, rule Rule) error {
	if !IsUserScoped(ex) {
		return xerrors.Errorf("experiment %q does not accept runtime rules", ex)
	}
	if err := checkShape(rule); err != nil {
		return err
	}
	if rule.Mode != ModeCondition {
		return nil
	}
	_, err := compileCondition(rule.Condition)
	return err
}

// checkShape verifies the mode and that a condition is present only for
// ModeCondition.
func checkShape(rule Rule) error {
	switch rule.Mode {
	case ModeInherit, ModeOn, ModeOff:
		if rule.Condition != "" {
			return xerrors.Errorf("mode %q must not have a condition", rule.Mode)
		}
	case ModeCondition:
		if rule.Condition == "" {
			return xerrors.New("mode \"condition\" requires a condition")
		}
	default:
		return xerrors.Errorf("unknown mode %q", rule.Mode)
	}
	return nil
}

// decodeRule decodes and shape-checks a stored rule. Errors must not be
// logged because JSON errors can quote stored content.
func decodeRule(stored StoredRule) (Rule, error) {
	var rule Rule
	if err := json.Unmarshal(stored.Value, &rule); err != nil {
		return Rule{}, xerrors.Errorf("decode rule: %w", err)
	}
	if err := checkShape(rule); err != nil {
		return rule, err
	}
	return rule, nil
}
