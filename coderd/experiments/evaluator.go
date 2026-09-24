package experiments

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

// Store supplies stored rules and subject attributes to the Evaluator.
type Store interface {
	// Rules returns the stored rule for every configured experiment.
	// Experiments without a rule are absent from the map.
	Rules(ctx context.Context) (map[codersdk.Experiment]StoredRule, error)
	// UserAttributes loads the attributes conditions can read for userID.
	UserAttributes(ctx context.Context, userID uuid.UUID) (User, error)
}

// Evaluator decides whether experiments are enabled for a subject user.
// It is safe for concurrent use. It does not cache decisions: every call
// reads the rules once, so rule changes apply to the next call.
type Evaluator struct {
	logger   slog.Logger
	store    Store
	static   codersdk.Experiments
	programs *programCache
}

// New returns an Evaluator. static is the startup --experiments list; it
// is the default for experiments whose rule is absent or inherits.
func New(logger slog.Logger, store Store, static codersdk.Experiments) (*Evaluator, error) {
	if store == nil {
		return nil, xerrors.New("experiments: store is required")
	}
	// Build the CEL environment now so a broken environment fails startup
	// instead of every decision.
	if _, err := celEnv(); err != nil {
		return nil, xerrors.Errorf("experiments: create CEL environment: %w", err)
	}
	return &Evaluator{
		logger:   logger,
		store:    store,
		static:   slices.Clone(static),
		programs: newProgramCache(),
	}, nil
}

// Enabled reports whether ex is enabled for userID.
func (e *Evaluator) Enabled(ctx context.Context, userID uuid.UUID, ex codersdk.Experiment) bool {
	if !IsUserScoped(ex) {
		return e.static.Enabled(ex)
	}
	return e.newDecision(ctx, userID).enabled(ex)
}

// EnabledExperiments returns the experiments enabled for userID. Static
// entries keep their order, including unknown strings. User-scoped
// entries decided off are removed, and user-scoped entries decided on that
// are not in the static list are appended in ExperimentsKnown order. Rules
// and attributes are each loaded at most once.
func (e *Evaluator) EnabledExperiments(ctx context.Context, userID uuid.UUID) codersdk.Experiments {
	d := e.newDecision(ctx, userID)
	result := make(codersdk.Experiments, 0, len(e.static))
	for _, ex := range e.static {
		if IsUserScoped(ex) && !d.enabled(ex) {
			continue
		}
		result = append(result, ex)
	}
	for _, ex := range codersdk.ExperimentsKnown {
		if !IsUserScoped(ex) || e.static.Enabled(ex) {
			continue
		}
		if d.enabled(ex) {
			result = append(result, ex)
		}
	}
	return result
}

// decision holds the state of one Enabled or EnabledExperiments call. It
// is not safe for concurrent use.
type decision struct {
	e      *Evaluator
	ctx    context.Context
	userID uuid.UUID

	// failed is set when every user-scoped decision in this call must be
	// off: a nil subject or a rules read error.
	failed bool
	rules  map[codersdk.Experiment]StoredRule

	userLoaded bool
	user       User
	userErr    error

	// decided memoizes per-experiment results within this call.
	decided map[codersdk.Experiment]bool
}

func (e *Evaluator) newDecision(ctx context.Context, userID uuid.UUID) *decision {
	d := &decision{e: e, ctx: ctx, userID: userID, decided: make(map[codersdk.Experiment]bool)}
	if userID == uuid.Nil {
		e.logger.Error(ctx, "experiment decision without a subject user; user-scoped experiments are off")
		d.failed = true
		return d
	}
	rules, err := e.store.Rules(ctx)
	if err != nil {
		e.logger.Error(ctx, "read experiment rules failed; user-scoped experiments are off",
			slog.F("category", categoryRead), slog.Error(err))
		d.failed = true
		return d
	}
	d.rules = rules
	return d
}

func (d *decision) enabled(ex codersdk.Experiment) bool {
	if result, ok := d.decided[ex]; ok {
		return result
	}
	result := d.decide(ex)
	d.decided[ex] = result
	return result
}

// decide applies the rule order: failed call, no rule or inherit, malformed
// rule, on or off, then condition.
func (d *decision) decide(ex codersdk.Experiment) bool {
	if d.failed {
		return false
	}
	stored, ok := d.rules[ex]
	if !ok {
		return d.e.static.Enabled(ex)
	}
	rule, err := decodeRule(stored)
	if err != nil {
		// Never log err: JSON errors can quote stored content.
		d.e.logger.Error(d.ctx, "experiment rule is malformed; experiment is off",
			slog.F("experiment", ex),
			slog.F("revision", rule.Revision),
			slog.F("category", categoryMalformed))
		return false
	}
	switch rule.Mode {
	case ModeInherit:
		return d.e.static.Enabled(ex)
	case ModeOn:
		return true
	case ModeOff:
		return false
	case ModeCondition:
		return d.evaluate(ex, rule)
	default:
		// decodeRule rejects unknown modes.
		panic("developer error: unhandled experiment rule mode " + string(rule.Mode))
	}
}

func (d *decision) evaluate(ex codersdk.Experiment, rule Rule) bool {
	user, err := d.loadUser()
	if err != nil {
		category := categoryRead
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			category = categoryCancelled
		}
		d.e.logger.Error(d.ctx, "load experiment subject attributes failed; experiment is off",
			slog.F("experiment", ex),
			slog.F("revision", rule.Revision),
			slog.F("category", category),
			slog.Error(err))
		return false
	}
	prg, err := d.e.programs.get(rule.Condition)
	if err == nil {
		var result bool
		result, err = evalCondition(d.ctx, prg, user)
		if err == nil {
			return result
		}
	}
	d.logConditionError(ex, rule, err)
	return false
}

func (d *decision) loadUser() (User, error) {
	if !d.userLoaded {
		d.userLoaded = true
		d.user, d.userErr = d.e.store.UserAttributes(d.ctx, d.userID)
	}
	return d.user, d.userErr
}

// logConditionError logs only the category and location. CEL messages can
// quote the condition, so err's text is never logged.
func (d *decision) logConditionError(ex codersdk.Experiment, rule Rule, err error) {
	fields := []slog.Field{
		slog.F("experiment", ex),
		slog.F("revision", rule.Revision),
	}
	var cerr *conditionError
	if !errors.As(err, &cerr) {
		// Only environment setup errors reach here; they contain no
		// condition text.
		d.e.logger.Error(d.ctx, "experiment condition failed; experiment is off", append(fields, slog.Error(err))...)
		return
	}
	fields = append(fields,
		slog.F("category", cerr.category),
		slog.F("line", cerr.line),
		slog.F("column", cerr.column))
	if cerr.category == categoryCancelled {
		d.e.logger.Debug(d.ctx, "experiment condition canceled; experiment is off", fields...)
		return
	}
	d.e.logger.Error(d.ctx, "experiment condition failed; experiment is off", fields...)
}
