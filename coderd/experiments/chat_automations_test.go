package experiments_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/experiments"
	"github.com/coder/coder/v2/coderd/experiments/experimentstest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const chatAutomations = codersdk.ExperimentChatAutomations

func TestChatAutomationsRegistration(t *testing.T) {
	t.Parallel()

	require.True(t, experiments.IsUserScoped(chatAutomations))
	require.Contains(t, codersdk.ExperimentsKnown, chatAutomations)
	// --experiments='*' enables the safe experiments. Automations act with
	// their owner's authority, so a wildcard must not turn them on.
	require.NotContains(t, codersdk.ExperimentsSafe, chatAutomations)

	for _, rule := range []experiments.Rule{
		{Mode: experiments.ModeInherit},
		{Mode: experiments.ModeOn},
		{Mode: experiments.ModeOff},
		{Mode: experiments.ModeCondition, Condition: `user.username == "alice"`},
	} {
		require.NoError(t, experiments.ValidateRule(chatAutomations, rule), "mode %q", rule.Mode)
	}
}

func TestChatAutomationsDecidedPerUser(t *testing.T) {
	t.Parallel()

	alice, bob := uuid.New(), uuid.New()
	users := map[uuid.UUID]experiments.User{
		alice: {ID: alice.String(), Username: "alice"},
		bob:   {ID: bob.String(), Username: "bob"},
	}

	cases := map[string]struct {
		static    bool
		rule      *experiments.Rule
		wantAlice bool
		wantBob   bool
	}{
		"static on, no rule":       {static: true, wantAlice: true, wantBob: true},
		"static off, no rule":      {},
		"inherit with static on":   {static: true, rule: &experiments.Rule{Mode: experiments.ModeInherit}, wantAlice: true, wantBob: true},
		"inherit with static off":  {rule: &experiments.Rule{Mode: experiments.ModeInherit}},
		"stored on, static off":    {rule: &experiments.Rule{Mode: experiments.ModeOn}, wantAlice: true, wantBob: true},
		"stored off, static on":    {static: true, rule: &experiments.Rule{Mode: experiments.ModeOff}},
		"condition, static off":    {rule: condition(`user.username == "alice"`), wantAlice: true},
		"condition, static on":     {static: true, rule: condition(`user.username == "alice"`), wantAlice: true},
		"condition turns off bob":  {static: true, rule: condition(`user.username != "bob"`), wantAlice: true},
		"condition on a user's ID": {rule: condition(`user.id == "` + bob.String() + `"`), wantBob: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			store := experimentstest.Store{Users: users}
			if tc.rule != nil {
				store.StoredRules = map[codersdk.Experiment]experiments.StoredRule{
					chatAutomations: experimentstest.StoredRule(t, *tc.rule),
				}
			}
			var static codersdk.Experiments
			if tc.static {
				static = codersdk.Experiments{chatAutomations}
			}
			e, _ := newEvaluator(t, store, static)

			require.Equal(t, tc.wantAlice, e.Enabled(ctx, alice, chatAutomations), "alice")
			require.Equal(t, tc.wantBob, e.Enabled(ctx, bob, chatAutomations), "bob")
		})
	}
}

// TestChatAutomationsFailClosed enables the experiment at startup, so only
// the failure itself can turn it off.
func TestChatAutomationsFailClosed(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	user := experiments.User{ID: userID.String(), Username: "alice"}
	rawRule := func(raw string) experimentstest.Store {
		return experimentstest.Store{
			Users:       map[uuid.UUID]experiments.User{userID: user},
			StoredRules: map[codersdk.Experiment]experiments.StoredRule{chatAutomations: {Value: []byte(raw)}},
		}
	}

	cases := map[string]struct {
		store   experimentstest.Store
		subject uuid.UUID
	}{
		"rules read error": {
			store:   experimentstest.Store{RulesErr: xerrors.New("db down")},
			subject: userID,
		},
		"malformed rule": {
			store:   rawRule(`{"mode": "on"`),
			subject: userID,
		},
		"unknown mode": {
			store:   rawRule(`{"mode": "percent", "revision": 2}`),
			subject: userID,
		},
		"attribute read error": {
			store: experimentstest.Store{
				StoredRules: map[codersdk.Experiment]experiments.StoredRule{
					chatAutomations: experimentstest.StoredRule(t, *condition(`true`)),
				},
				UserErr: xerrors.New("db down"),
			},
			subject: userID,
		},
		"condition evaluation error": {
			store: experimentstest.Store{
				Users: map[uuid.UUID]experiments.User{userID: user},
				StoredRules: map[codersdk.Experiment]experiments.StoredRule{
					chatAutomations: experimentstest.StoredRule(t, *condition(`user.roles[5] == "owner"`)),
				},
			},
			subject: userID,
		},
		"nil subject": {
			store: experimentstest.Store{
				Users: map[uuid.UUID]experiments.User{userID: user},
				StoredRules: map[codersdk.Experiment]experiments.StoredRule{
					chatAutomations: experimentstest.StoredRule(t, experiments.Rule{Mode: experiments.ModeOn}),
				},
			},
			subject: uuid.Nil,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			e, sink := newEvaluator(t, tc.store, codersdk.Experiments{chatAutomations})

			require.False(t, e.Enabled(ctx, tc.subject, chatAutomations))
			require.False(t, slices.Contains(e.EnabledExperiments(ctx, tc.subject), chatAutomations))
			require.NotEmpty(t, sink.Entries(), "fail-closed decisions must be logged")
		})
	}
}
