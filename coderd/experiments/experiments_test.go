package experiments_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/experiments"
	"github.com/coder/coder/v2/coderd/experiments/experimentstest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const (
	scoped   = codersdk.ExperimentExample
	unscoped = codersdk.ExperimentAutoFillParameters
	sentinel = "sentinel-7f3a@example.test"
)

// countingStore records how often each Store method runs.
type countingStore struct {
	experiments.Store
	rules atomic.Int64
	users atomic.Int64
}

func (s *countingStore) Rules(ctx context.Context) (map[codersdk.Experiment]experiments.StoredRule, error) {
	s.rules.Add(1)
	return s.Store.Rules(ctx)
}

func (s *countingStore) UserAttributes(ctx context.Context, userID uuid.UUID) (experiments.User, error) {
	s.users.Add(1)
	return s.Store.UserAttributes(ctx, userID)
}

var testUser = experiments.User{
	ID:            "6d8e1a52-6f2c-4a4e-9b3a-0d1d8f1f7f10",
	Username:      "alice",
	Email:         "Alice@Example.com",
	Roles:         []string{"owner"},
	Organizations: []string{"coder", "acme"},
	Groups:        []string{"coder/Everyone", "coder/beta", "acme/Everyone"},
}

func storeWith(t *testing.T, userID uuid.UUID, rule *experiments.Rule) *countingStore {
	t.Helper()
	s := experimentstest.Store{Users: map[uuid.UUID]experiments.User{userID: testUser}}
	if rule != nil {
		s.StoredRules = map[codersdk.Experiment]experiments.StoredRule{
			scoped: experimentstest.StoredRule(t, *rule),
		}
	}
	return &countingStore{Store: s}
}

func rawStore(userID uuid.UUID, raw string) *countingStore {
	return &countingStore{Store: experimentstest.Store{
		Users:       map[uuid.UUID]experiments.User{userID: testUser},
		StoredRules: map[codersdk.Experiment]experiments.StoredRule{scoped: {Value: []byte(raw)}},
	}}
}

func newEvaluator(t *testing.T, store experiments.Store, static codersdk.Experiments) (*experiments.Evaluator, *testutil.FakeSink) {
	t.Helper()
	sink := testutil.NewFakeSink(t)
	e, err := experiments.New(sink.Logger(), store, static)
	require.NoError(t, err)
	return e, sink
}

func condition(src string) *experiments.Rule {
	return &experiments.Rule{Mode: experiments.ModeCondition, Condition: src, Revision: 3}
}

// requireLogsExclude fails if any captured entry contains one of the
// forbidden strings in its message or fields.
func requireLogsExclude(t *testing.T, sink *testutil.FakeSink, forbidden ...string) {
	t.Helper()
	for _, entry := range sink.Entries() {
		text := entry.Message + " " + fmt.Sprint(entry.Fields)
		for _, f := range forbidden {
			require.NotContains(t, text, f)
		}
	}
}

// requireLogField fails unless some captured entry has field name set to
// want.
func requireLogField(t *testing.T, sink *testutil.FakeSink, name string, want any) {
	t.Helper()
	for _, entry := range sink.Entries() {
		for _, field := range entry.Fields {
			if field.Name == name && fmt.Sprint(field.Value) == fmt.Sprint(want) {
				return
			}
		}
	}
	require.Failf(t, "missing log field", "%s=%v", name, want)
}

func TestNew(t *testing.T) {
	t.Parallel()

	_, err := experiments.New(slog.Make(), nil, nil)
	require.Error(t, err)
}

func TestValidateRule(t *testing.T) {
	t.Parallel()

	valid := []experiments.Rule{
		{Mode: experiments.ModeInherit},
		{Mode: experiments.ModeOn},
		{Mode: experiments.ModeOff},
		{Mode: experiments.ModeCondition, Condition: `user.email == "a@b.c" || "admin" in user.roles`},
		{Mode: experiments.ModeCondition, Condition: strings.Repeat(" ", experiments.MaxConditionSize-4) + "true"},
	}
	for _, rule := range valid {
		require.NoError(t, experiments.ValidateRule(scoped, rule), "mode %q", rule.Mode)
	}

	invalid := map[string]experiments.Rule{
		"syntax":          {Mode: experiments.ModeCondition, Condition: `user.email ==`},
		"non-bool":        {Mode: experiments.ModeCondition, Condition: `user.email`},
		"unknown field":   {Mode: experiments.ModeCondition, Condition: `user.nickname == "x"`},
		"unknown var":     {Mode: experiments.ModeCondition, Condition: `request.ip == "x"`},
		"size":            {Mode: experiments.ModeCondition, Condition: strings.Repeat(" ", experiments.MaxConditionSize-3) + "true"},
		"empty":           {Mode: experiments.ModeCondition},
		"unknown mode":    {Mode: "sometimes"},
		"stray condition": {Mode: experiments.ModeOn, Condition: "true"},
	}
	for name, rule := range invalid {
		require.Error(t, experiments.ValidateRule(scoped, rule), name)
	}

	require.Error(t, experiments.ValidateRule(unscoped, experiments.Rule{Mode: experiments.ModeOn}))
}

func TestDiagnosticsNotLogged(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		src         string
		compileFail bool
		// message is part of the CEL message text for this failure.
		message  string
		category string
	}{
		"syntax":  {src: `user.email == "` + sentinel + `" &&`, compileFail: true, message: "Syntax error", category: "syntax"},
		"type":    {src: `user.email == 1 || user.email == "` + sentinel + `"`, compileFail: true, message: "no matching overload", category: "type"},
		"runtime": {src: `user.email == "x" || int("` + sentinel + `") == 1`, message: "conversion error", category: "eval"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			userID := uuid.New()

			validateErr := experiments.ValidateRule(scoped, *condition(tc.src))
			if tc.compileFail {
				require.Error(t, validateErr)
			} else {
				require.NoError(t, validateErr)
			}

			e, sink := newEvaluator(t, storeWith(t, userID, condition(tc.src)), codersdk.Experiments{scoped})
			require.False(t, e.Enabled(ctx, userID, scoped))
			require.NotEmpty(t, sink.Entries(), "failure must be logged")
			forbidden := []string{sentinel, tc.src, tc.message}
			if validateErr != nil {
				require.Contains(t, validateErr.Error(), tc.message)
				forbidden = append(forbidden, validateErr.Error())
			}
			requireLogsExclude(t, sink, forbidden...)
			requireLogField(t, sink, "category", tc.category)
			requireLogField(t, sink, "experiment", scoped)
			requireLogField(t, sink, "revision", int64(3))
		})
	}
}

func TestDecisions(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		rule          *experiments.Rule
		static        bool
		want          bool
		wantUserLoads int64
	}{
		"none static on":     {static: true, want: true},
		"none static off":    {static: false, want: false},
		"inherit static on":  {rule: &experiments.Rule{Mode: experiments.ModeInherit}, static: true, want: true},
		"inherit static off": {rule: &experiments.Rule{Mode: experiments.ModeInherit}, static: false, want: false},
		"on":                 {rule: &experiments.Rule{Mode: experiments.ModeOn}, want: true},
		"off static on":      {rule: &experiments.Rule{Mode: experiments.ModeOff}, static: true, want: false},
		"email":              {rule: condition(`user.email == "Alice@Example.com"`), want: true, wantUserLoads: 1},
		"email case":         {rule: condition(`user.email == "alice@example.com"`), static: true, want: false, wantUserLoads: 1},
		"username id":        {rule: condition(`user.username == "alice" && user.id == "` + testUser.ID + `"`), want: true, wantUserLoads: 1},
		"role":               {rule: condition(`"owner" in user.roles`), want: true, wantUserLoads: 1},
		"missing role":       {rule: condition(`"member" in user.roles`), want: false, wantUserLoads: 1},
		"org":                {rule: condition(`"acme" in user.organizations`), want: true, wantUserLoads: 1},
		"group":              {rule: condition(`"coder/beta" in user.groups`), want: true, wantUserLoads: 1},
		"everyone":           {rule: condition(`"acme/Everyone" in user.groups`), want: true, wantUserLoads: 1},
		"and":                {rule: condition(`"coder" in user.organizations && "acme/beta" in user.groups`), want: false, wantUserLoads: 1},
		"or":                 {rule: condition(`"other" in user.organizations || "coder/beta" in user.groups`), want: true, wantUserLoads: 1},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			userID := uuid.New()
			store := storeWith(t, userID, tc.rule)
			var static codersdk.Experiments
			if tc.static {
				static = codersdk.Experiments{scoped}
			}
			e, _ := newEvaluator(t, store, static)
			require.Equal(t, tc.want, e.Enabled(ctx, userID, scoped))
			require.EqualValues(t, 1, store.rules.Load())
			require.Equal(t, tc.wantUserLoads, store.users.Load())
		})
	}
}

func TestUnscopedUsesStatic(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	// Rules for experiments outside ExperimentsUserScoped are inert, and
	// the subject does not matter.
	store := &countingStore{Store: experimentstest.Store{
		RulesErr: xerrors.New("boom"),
	}}
	e, _ := newEvaluator(t, store, codersdk.Experiments{unscoped})
	require.True(t, e.Enabled(ctx, uuid.Nil, unscoped))
	require.EqualValues(t, 0, store.rules.Load())

	store = &countingStore{Store: experimentstest.Store{StoredRules: map[codersdk.Experiment]experiments.StoredRule{
		unscoped: experimentstest.StoredRule(t, experiments.Rule{Mode: experiments.ModeOff}),
	}}}
	e, _ = newEvaluator(t, store, codersdk.Experiments{unscoped})
	require.True(t, e.Enabled(ctx, uuid.New(), unscoped))
}

func TestFailClosed(t *testing.T) {
	t.Parallel()

	// manyGroups makes nested comprehensions exceed the cost limit.
	manyGroups := make([]string, 200)
	for i := range manyGroups {
		manyGroups[i] = fmt.Sprintf("coder/g%d", i)
	}

	cases := map[string]struct {
		store      func(t *testing.T, userID uuid.UUID) experiments.Store
		ctx        func(ctx context.Context) context.Context
		nilSubject bool
	}{
		"rules read error": {store: func(_ *testing.T, _ uuid.UUID) experiments.Store {
			return experimentstest.Store{RulesErr: xerrors.New("db down")}
		}},
		"malformed json": {store: func(_ *testing.T, userID uuid.UUID) experiments.Store {
			return rawStore(userID, `{"mode": "inherit"`)
		}},
		"unknown mode": {store: func(_ *testing.T, userID uuid.UUID) experiments.Store {
			return rawStore(userID, `{"mode": "percent", "revision": 2}`)
		}},
		"condition without text": {store: func(_ *testing.T, userID uuid.UUID) experiments.Store {
			return rawStore(userID, `{"mode": "condition", "revision": 2}`)
		}},
		"attribute load error": {store: func(t *testing.T, _ uuid.UUID) experiments.Store {
			return experimentstest.Store{
				StoredRules: map[codersdk.Experiment]experiments.StoredRule{
					scoped: experimentstest.StoredRule(t, *condition(`true`)),
				},
				UserErr: xerrors.New("db down"),
			}
		}},
		"compile error": {store: func(t *testing.T, userID uuid.UUID) experiments.Store {
			return storeWith(t, userID, condition(`user.nickname == "x"`))
		}},
		"runtime error": {store: func(t *testing.T, userID uuid.UUID) experiments.Store {
			return storeWith(t, userID, condition(`user.roles[5] == "owner"`))
		}},
		"cost overrun": {store: func(t *testing.T, userID uuid.UUID) experiments.Store {
			return experimentstest.Store{
				StoredRules: map[codersdk.Experiment]experiments.StoredRule{
					scoped: experimentstest.StoredRule(t, *condition(`user.groups.all(a, user.groups.all(b, a != b || true))`)),
				},
				Users: map[uuid.UUID]experiments.User{userID: {Groups: manyGroups}},
			}
		}},
		"canceled context": {
			store: func(t *testing.T, userID uuid.UUID) experiments.Store {
				return storeWith(t, userID, condition(`true`))
			},
			ctx: func(ctx context.Context) context.Context {
				ctx, cancel := context.WithCancel(ctx)
				cancel()
				return ctx
			},
		},
		"nil subject": {
			store: func(t *testing.T, userID uuid.UUID) experiments.Store {
				return storeWith(t, userID, &experiments.Rule{Mode: experiments.ModeInherit})
			},
			nilSubject: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			if tc.ctx != nil {
				ctx = tc.ctx(ctx)
			}
			userID := uuid.New()
			store := tc.store(t, userID)
			if tc.nilSubject {
				userID = uuid.Nil
			}
			e, sink := newEvaluator(t, store, codersdk.Experiments{scoped, unscoped})
			require.False(t, e.Enabled(ctx, userID, scoped))
			require.Equal(t, codersdk.Experiments{unscoped}, e.EnabledExperiments(ctx, userID))
			require.NotEmpty(t, sink.Entries(), "fail-closed decisions must be logged")
		})
	}
}

// TestRuleChangesApplyToNextCall edits the stored rule between calls on one
// Evaluator: each call must reflect the current rule, never an earlier
// program or decision, including after a failed read.
func TestRuleChangesApplyToNextCall(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	userID := uuid.New()
	store := &swapStore{Store: storeWith(t, userID, nil)}
	e, _ := newEvaluator(t, store, nil)

	steps := []struct {
		name string
		// rule nil means the rules read fails.
		rule *experiments.Rule
		want bool
	}{
		{"matching condition", condition(`"owner" in user.roles`), true},
		{"edited condition", condition(`"member" in user.roles`), false},
		{"original condition again", condition(`"owner" in user.roles`), true},
		{"read failure", nil, false},
		{"after read failure", condition(`"owner" in user.roles`), true},
		{"off", &experiments.Rule{Mode: experiments.ModeOff}, false},
		{"on", &experiments.Rule{Mode: experiments.ModeOn}, true},
	}
	for _, step := range steps {
		if step.rule == nil {
			store.rules.Store(nil)
		} else {
			rules := map[codersdk.Experiment]experiments.StoredRule{
				scoped: experimentstest.StoredRule(t, *step.rule),
			}
			store.rules.Store(&rules)
		}
		require.Equal(t, step.want, e.Enabled(ctx, userID, scoped), step.name)
	}
}

// swapStore returns the rules currently stored in rules, and fails the read
// while rules is nil.
type swapStore struct {
	experiments.Store
	rules atomic.Pointer[map[codersdk.Experiment]experiments.StoredRule]
}

func (s *swapStore) Rules(context.Context) (map[codersdk.Experiment]experiments.StoredRule, error) {
	rules := s.rules.Load()
	if rules == nil {
		return nil, xerrors.New("db down")
	}
	return *rules, nil
}

func TestEnabledExperiments(t *testing.T) {
	t.Parallel()

	unknown := codersdk.Experiment("not-a-real-experiment")

	t.Run("KeepsStaticOrderAndUnknown", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		userID := uuid.New()
		static := codersdk.Experiments{unknown, scoped, unscoped}
		store := storeWith(t, userID, nil)
		e, _ := newEvaluator(t, store, static)
		require.Equal(t, static, e.EnabledExperiments(ctx, userID))
		require.EqualValues(t, 1, store.rules.Load())
		require.EqualValues(t, 0, store.users.Load())
	})

	t.Run("RemovesDecidedOff", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		userID := uuid.New()
		store := storeWith(t, userID, condition(`"nobody" in user.roles`))
		e, _ := newEvaluator(t, store, codersdk.Experiments{unknown, scoped, unscoped})
		require.Equal(t, codersdk.Experiments{unknown, unscoped}, e.EnabledExperiments(ctx, userID))
		require.EqualValues(t, 1, store.rules.Load())
		require.EqualValues(t, 1, store.users.Load())
	})

	t.Run("AppendsDecidedOn", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		userID := uuid.New()
		store := storeWith(t, userID, condition(`"owner" in user.roles`))
		e, _ := newEvaluator(t, store, codersdk.Experiments{unscoped, unknown})
		require.Equal(t, codersdk.Experiments{unscoped, unknown, scoped}, e.EnabledExperiments(ctx, userID))
		require.EqualValues(t, 1, store.rules.Load())
		require.EqualValues(t, 1, store.users.Load())
	})
}

func TestConcurrentEvaluation(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitMedium)
	userID := uuid.New()

	sources := []string{`"owner" in user.roles`, `"member" in user.roles`, `user.username == "alice"`}
	wants := []bool{true, false, true}
	rules := make([]map[codersdk.Experiment]experiments.StoredRule, len(sources))
	for i, src := range sources {
		rules[i] = map[codersdk.Experiment]experiments.StoredRule{
			scoped: experimentstest.StoredRule(t, *condition(src)),
		}
	}
	// Each goroutine selects its rule through the context, so all of them
	// share one evaluator and its program cache.
	store := &ctxStore{Store: storeWith(t, userID, nil), rules: rules}
	e, _ := newEvaluator(t, store, nil)

	var wg sync.WaitGroup
	for g := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			i := g % len(sources)
			gctx := context.WithValue(ctx, ruleIndexKey{}, i)
			for range 50 {
				assert.Equal(t, wants[i], e.Enabled(gctx, userID, scoped), sources[i])
			}
		}()
	}
	wg.Wait()
}

type ruleIndexKey struct{}

// ctxStore returns the rules selected by the ruleIndexKey context value.
type ctxStore struct {
	experiments.Store
	rules []map[codersdk.Experiment]experiments.StoredRule
}

func (s *ctxStore) Rules(ctx context.Context) (map[codersdk.Experiment]experiments.StoredRule, error) {
	i, ok := ctx.Value(ruleIndexKey{}).(int)
	if !ok {
		return nil, xerrors.New("missing rule index")
	}
	return s.rules[i], nil
}
