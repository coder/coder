package experiments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
)

// ruleLockPrefix names the advisory lock that serializes writes to one
// experiment's rule. It matches the site_configs key prefix.
const ruleLockPrefix = "experiment_rule:"

// writeLockTimeout bounds how long WriteRule waits for a concurrent writer
// of the same experiment.
const writeLockTimeout = 5 * time.Second

// RevisionConflictError is returned by WriteRule when the expected revision
// is not the current revision of the stored rule.
type RevisionConflictError struct {
	Experiment codersdk.Experiment
	Expected   int64
	// Current is the stored revision; zero means no rule is stored.
	Current int64
}

func (e *RevisionConflictError) Error() string {
	return fmt.Sprintf("experiment %q rule is at revision %d, expected revision %d", e.Experiment, e.Current, e.Expected)
}

// WriteRule stores the mode and condition of rule as the rule for ex when
// the stored revision equals expectedRevision. The Revision, UpdatedBy and
// UpdatedAt fields of rule are ignored: WriteRule sets them.
//
// It returns the stored rule before and after the call, as read and written
// inside one transaction. Before the first write of ex, the old rule is the
// zero Rule (revision 0). When the stored mode and condition already equal
// rule's, nothing is written, changed is false and the new rule equals the
// old one. Writing inherit to an experiment without a row stores revision 1
// and counts as a change.
//
// A stale expectedRevision returns *RevisionConflictError. Validation errors
// can contain CEL diagnostics; return them only to the rule author and never
// log them.
func WriteRule(
	ctx context.Context,
	db database.Store,
	actorID uuid.UUID,
	ex codersdk.Experiment,
	rule Rule,
	expectedRevision int64,
) (oldRule, newRule Rule, changed bool, err error) {
	if actorID == uuid.Nil {
		return Rule{}, Rule{}, false, xerrors.New("write experiment rule: actor is required")
	}
	if expectedRevision < 0 {
		return Rule{}, Rule{}, false, xerrors.Errorf("write experiment rule: negative expected revision %d", expectedRevision)
	}
	if err := ValidateRule(ex, rule); err != nil {
		return Rule{}, Rule{}, false, xerrors.Errorf("invalid experiment rule: %w", err)
	}

	lockCtx, cancel := context.WithTimeout(ctx, writeLockTimeout)
	defer cancel()
	err = db.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(lockCtx, database.GenLockID(ruleLockPrefix+string(ex))); err != nil {
			return xerrors.Errorf("acquire experiment rule lock: %w", err)
		}
		current, err := readRule(ctx, tx, ex)
		if err != nil {
			return err
		}
		if current.Revision != expectedRevision {
			return &RevisionConflictError{Experiment: ex, Expected: expectedRevision, Current: current.Revision}
		}
		oldRule = current
		if current.Revision > 0 && current.Mode == rule.Mode && current.Condition == rule.Condition {
			newRule = current
			return nil
		}
		newRule = Rule{
			Mode:      rule.Mode,
			Condition: rule.Condition,
			Revision:  current.Revision + 1,
			UpdatedBy: actorID,
			UpdatedAt: dbtime.Now(),
		}
		value, err := json.Marshal(newRule)
		if err != nil {
			return xerrors.Errorf("encode experiment rule: %w", err)
		}
		if err := tx.UpsertExperimentRule(ctx, database.UpsertExperimentRuleParams{
			Experiment: string(ex),
			Value:      string(value),
		}); err != nil {
			return xerrors.Errorf("upsert experiment rule: %w", err)
		}
		changed = true
		return nil
	}, &database.TxOptions{Isolation: sql.LevelReadCommitted, TxIdentifier: "experiment_rule_write"})
	if err != nil {
		return Rule{}, Rule{}, false, err
	}
	return oldRule, newRule, changed, nil
}

// auditIDNamespace derives audit resource IDs from experiment names. It
// must never change: audit history groups entries by these IDs.
var auditIDNamespace = uuid.MustParse("65b7b16d-c22d-4972-adca-014b0a4b77f0")

// AuditRecord returns the audited form of rule for ex. The ID is derived
// from the experiment name, so every audit entry for one experiment shares
// it. A zero Rule (no stored rule) yields empty mode and condition with
// revision 0.
func AuditRecord(ex codersdk.Experiment, rule Rule) database.ExperimentRule {
	return database.ExperimentRule{
		ID:         uuid.NewSHA1(auditIDNamespace, []byte(ex)),
		Experiment: string(ex),
		Mode:       string(rule.Mode),
		Condition:  rule.Condition,
		Revision:   rule.Revision,
	}
}

// readRule returns the stored rule for ex, or the zero Rule when none is
// stored. Only the revision must be readable, so that a rule the Evaluator
// treats as malformed can still be replaced: for such a value readRule
// returns a Rule with only Revision set, which never equals a valid rule.
func readRule(ctx context.Context, tx database.Store, ex codersdk.Experiment) (Rule, error) {
	value, err := tx.GetExperimentRule(ctx, string(ex))
	if errors.Is(err, sql.ErrNoRows) {
		return Rule{}, nil
	}
	if err != nil {
		return Rule{}, xerrors.Errorf("get experiment rule: %w", err)
	}
	var revision struct {
		Revision int64 `json:"revision"`
	}
	// Decode errors are dropped because they can quote stored content.
	if err := json.Unmarshal([]byte(value), &revision); err != nil {
		return Rule{}, xerrors.Errorf("stored rule for experiment %q has no readable revision", ex)
	}
	if revision.Revision < 1 {
		// WriteRule always stores revision 1 or higher.
		return Rule{}, xerrors.Errorf("stored rule for experiment %q has invalid revision %d", ex, revision.Revision)
	}
	var rule Rule
	if err := json.Unmarshal([]byte(value), &rule); err != nil || checkShape(rule) != nil {
		return Rule{Revision: revision.Revision}, nil
	}
	return rule, nil
}
