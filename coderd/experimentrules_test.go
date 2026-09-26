package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// ruleWriter writes experiment rules through the experimental API and
// tracks the revision of each experiment so that every write expects the
// current revision.
type ruleWriter struct {
	t         *testing.T
	client    *codersdk.ExperimentalClient
	revisions map[codersdk.Experiment]int64
}

func newRuleWriter(t *testing.T, client *codersdk.Client) *ruleWriter {
	return &ruleWriter{t: t, client: codersdk.NewExperimentalClient(client), revisions: map[codersdk.Experiment]int64{}}
}

func (w *ruleWriter) put(ctx context.Context, ex codersdk.Experiment, mode codersdk.ExperimentRuleMode, condition string) codersdk.ExperimentRule {
	w.t.Helper()
	rule, err := w.client.PutExperimentRule(ctx, ex, codersdk.PutExperimentRuleRequest{
		Mode:             mode,
		Condition:        condition,
		ExpectedRevision: w.revisions[ex],
	})
	require.NoError(w.t, err)
	require.Equal(w.t, w.revisions[ex]+1, rule.Revision)
	w.revisions[ex] = rule.Revision
	return rule
}

func requireEnabled(ctx context.Context, t *testing.T, client *codersdk.Client, ex codersdk.Experiment, want bool) {
	t.Helper()
	got, err := client.Experiments(ctx)
	require.NoError(t, err)
	require.Equal(t, want, got.Enabled(ex), "experiment %q for the caller", ex)
}

func requireStatus(t *testing.T, err error, status int) *codersdk.Error {
	t.Helper()
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, status, sdkErr.StatusCode(), sdkErr.Error())
	return sdkErr
}

func ruleEntry(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, ex codersdk.Experiment) codersdk.ExperimentRuleEntry {
	t.Helper()
	entries, err := client.ExperimentRules(ctx)
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.Experiment == string(ex) {
			return entry
		}
	}
	t.Fatalf("no rules entry for experiment %q", ex)
	return codersdk.ExperimentRuleEntry{}
}

func TestExperimentRules(t *testing.T) {
	t.Parallel()

	// Each write takes effect on the member's next request, without a
	// restart. mcp-tool-search is enabled at startup, example is not.
	t.Run("WritesApplyToNextRequest", func(t *testing.T) {
		t.Parallel()
		ownerClient := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: coderdtest.DeploymentValues(t, func(v *codersdk.DeploymentValues) {
				v.Experiments = []string{string(codersdk.ExperimentMCPToolSearch)}
			}),
		})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)
		w := newRuleWriter(t, ownerClient)

		entries, err := w.client.ExperimentRules(ctx)
		require.NoError(t, err)
		require.Equal(t, []codersdk.ExperimentRuleEntry{
			{Experiment: string(codersdk.ExperimentExample)},
			{Experiment: string(codersdk.ExperimentMCPToolSearch), StaticDefault: true},
		}, entries)

		w.put(ctx, codersdk.ExperimentExample, codersdk.ExperimentRuleModeOn, "")
		requireEnabled(ctx, t, memberClient, codersdk.ExperimentExample, true)

		w.put(ctx, codersdk.ExperimentExample, codersdk.ExperimentRuleModeCondition, fmt.Sprintf("user.username == %q", member.Username))
		requireEnabled(ctx, t, memberClient, codersdk.ExperimentExample, true)
		requireEnabled(ctx, t, ownerClient, codersdk.ExperimentExample, false)

		w.put(ctx, codersdk.ExperimentExample, codersdk.ExperimentRuleModeOff, "")
		requireEnabled(ctx, t, memberClient, codersdk.ExperimentExample, false)

		// off is a kill switch for a statically enabled experiment, and
		// reset restores the startup default.
		w.put(ctx, codersdk.ExperimentMCPToolSearch, codersdk.ExperimentRuleModeOff, "")
		requireEnabled(ctx, t, memberClient, codersdk.ExperimentMCPToolSearch, false)
		w.put(ctx, codersdk.ExperimentMCPToolSearch, codersdk.ExperimentRuleModeInherit, "")
		requireEnabled(ctx, t, memberClient, codersdk.ExperimentMCPToolSearch, true)

		entry := ruleEntry(ctx, t, w.client, codersdk.ExperimentMCPToolSearch)
		require.True(t, entry.StaticDefault)
		require.NotNil(t, entry.Rule)
		require.Equal(t, string(codersdk.ExperimentRuleModeInherit), entry.Rule.Mode)
		require.Equal(t, int64(2), entry.Rule.Revision)
		require.Equal(t, owner.UserID, entry.Rule.UpdatedBy)
		require.False(t, entry.Rule.UpdatedAt.IsZero())

		// Responses carry condition text and must never be cached.
		for _, req := range []struct {
			method, path string
			body         any
		}{
			{http.MethodGet, "/api/experimental/experiments/rules", nil},
			{http.MethodPut, "/api/experimental/experiments/rules/example", codersdk.PutExperimentRuleRequest{Mode: codersdk.ExperimentRuleModeOff, ExpectedRevision: 3}},
		} {
			res, err := ownerClient.Request(ctx, req.method, req.path, req.body)
			require.NoError(t, err)
			_ = res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode, req.method)
			require.Equal(t, "no-store", res.Header.Get("Cache-Control"), req.method)
		}
	})

	t.Run("RejectedWritesKeepRule", func(t *testing.T) {
		t.Parallel()
		ownerClient := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)
		w := newRuleWriter(t, ownerClient)
		stored := w.put(ctx, codersdk.ExperimentExample, codersdk.ExperimentRuleModeOn, "")

		put := func(ex codersdk.Experiment, req codersdk.PutExperimentRuleRequest) error {
			_, err := w.client.PutExperimentRule(ctx, ex, req)
			return err
		}

		// The author of an invalid condition receives the diagnostics.
		err := put(codersdk.ExperimentExample, codersdk.PutExperimentRuleRequest{Mode: codersdk.ExperimentRuleModeCondition, Condition: `user.username == "a" &&`, ExpectedRevision: 1})
		require.Contains(t, requireStatus(t, err, http.StatusBadRequest).Detail, "Syntax error")

		for _, req := range []codersdk.PutExperimentRuleRequest{
			{Mode: codersdk.ExperimentRuleModeOff, Condition: "true", ExpectedRevision: 1},
			{Mode: codersdk.ExperimentRuleModeCondition, ExpectedRevision: 1},
			{Mode: "percent", ExpectedRevision: 1},
			{Mode: codersdk.ExperimentRuleModeOff, ExpectedRevision: -1},
		} {
			requireStatus(t, put(codersdk.ExperimentExample, req), http.StatusBadRequest)
		}
		for _, ex := range []codersdk.Experiment{codersdk.ExperimentAutoFillParameters, "not-an-experiment"} {
			requireStatus(t, put(ex, codersdk.PutExperimentRuleRequest{Mode: codersdk.ExperimentRuleModeOn}), http.StatusBadRequest)
		}

		// A stale revision conflicts; the same rule at the current
		// revision is an accepted no-op.
		requireStatus(t, put(codersdk.ExperimentExample, codersdk.PutExperimentRuleRequest{Mode: codersdk.ExperimentRuleModeOff}), http.StatusConflict)
		same, err := w.client.PutExperimentRule(ctx, codersdk.ExperimentExample, codersdk.PutExperimentRuleRequest{Mode: codersdk.ExperimentRuleModeOn, ExpectedRevision: 1})
		require.NoError(t, err)
		require.Equal(t, stored, same)

		entries, err := w.client.ExperimentRules(ctx)
		require.NoError(t, err)
		require.Equal(t, []codersdk.ExperimentRuleEntry{
			{Experiment: string(codersdk.ExperimentExample), Rule: &stored},
			{Experiment: string(codersdk.ExperimentMCPToolSearch)},
		}, entries)
	})

	// Authorization runs before the body is parsed or compiled: a caller
	// without update access gets 403 even for an invalid condition.
	t.Run("Authorization", func(t *testing.T) {
		t.Parallel()
		ownerClient := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		auditorClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleAuditor())
		ctx := testutil.Context(t, testutil.WaitLong)

		member := codersdk.NewExperimentalClient(memberClient)
		_, err := member.ExperimentRules(ctx)
		requireStatus(t, err, http.StatusForbidden)

		auditor := codersdk.NewExperimentalClient(auditorClient)
		_, err = auditor.ExperimentRules(ctx)
		require.NoError(t, err)

		for _, client := range []*codersdk.ExperimentalClient{member, auditor} {
			for _, req := range []codersdk.PutExperimentRuleRequest{
				{Mode: codersdk.ExperimentRuleModeOn},
				{Mode: codersdk.ExperimentRuleModeCondition, Condition: "user."},
			} {
				_, err := client.PutExperimentRule(ctx, codersdk.ExperimentExample, req)
				requireStatus(t, err, http.StatusForbidden)
			}
		}
		require.Nil(t, ruleEntry(ctx, t, codersdk.NewExperimentalClient(ownerClient), codersdk.ExperimentExample).Rule)
	})

	// Stored rules the API cannot write are listed but ignored, and a
	// malformed rule of a user-scoped experiment stays replaceable.
	t.Run("StoredRulesOutsideTheAPI", func(t *testing.T) {
		t.Parallel()
		ownerClient, db := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)
		sysCtx := dbauthz.AsSystemRestricted(ctx)
		for ex, value := range map[codersdk.Experiment]string{
			"not-an-experiment":                   `{"mode":"on","revision":1}`,
			codersdk.ExperimentAutoFillParameters: `{"mode":"off","revision":4}`,
			codersdk.ExperimentExample:            `{"mode":"percent","revision":2}`,
		} {
			require.NoError(t, db.UpsertExperimentRule(sysCtx, database.UpsertExperimentRuleParams{Experiment: string(ex), Value: value}))
		}
		client := codersdk.NewExperimentalClient(ownerClient)

		entries, err := client.ExperimentRules(ctx)
		require.NoError(t, err)
		require.Equal(t, []codersdk.ExperimentRuleEntry{
			{Experiment: string(codersdk.ExperimentExample), Rule: &codersdk.ExperimentRule{Revision: 2}},
			{Experiment: string(codersdk.ExperimentMCPToolSearch)},
			{Experiment: string(codersdk.ExperimentAutoFillParameters), Rule: &codersdk.ExperimentRule{Mode: string(codersdk.ExperimentRuleModeOff), Revision: 4}, Ignored: true},
			{Experiment: "not-an-experiment", Rule: &codersdk.ExperimentRule{Mode: string(codersdk.ExperimentRuleModeOn), Revision: 1}, Ignored: true},
		}, entries)

		rule, err := client.PutExperimentRule(ctx, codersdk.ExperimentExample, codersdk.PutExperimentRuleRequest{Mode: codersdk.ExperimentRuleModeOn, ExpectedRevision: 2})
		require.NoError(t, err)
		require.Equal(t, int64(3), rule.Revision)
	})
}

// TestExperimentRulesReplicas checks that a rule written through one
// replica decides the next evaluation on another replica sharing the
// database, both for the API and for the evaluator chatd uses.
func TestExperimentRulesReplicas(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	options := func() *coderdtest.Options {
		return &coderdtest.Options{Database: db, Pubsub: ps}
	}
	clientA := coderdtest.New(t, options())
	clientB, _, apiB := coderdtest.NewWithAPI(t, options())
	owner := coderdtest.CreateFirstUser(t, clientA)
	memberA, member := coderdtest.CreateAnotherUser(t, clientA, owner.OrganizationID)
	memberB := codersdk.New(clientB.URL)
	memberB.SetSessionToken(memberA.SessionToken())
	ctx := testutil.Context(t, testutil.WaitLong)
	w := newRuleWriter(t, clientA)

	requireB := func(want bool) {
		t.Helper()
		requireEnabled(ctx, t, memberB, codersdk.ExperimentExample, want)
		require.Equal(t, want, apiB.ExperimentEvaluator.Enabled(ctx, member.ID, codersdk.ExperimentExample))
	}
	requireB(false)
	w.put(ctx, codersdk.ExperimentExample, codersdk.ExperimentRuleModeCondition, fmt.Sprintf("user.id == %q", member.ID))
	requireB(true)
	w.put(ctx, codersdk.ExperimentExample, codersdk.ExperimentRuleModeOff, "")
	requireB(false)
}
