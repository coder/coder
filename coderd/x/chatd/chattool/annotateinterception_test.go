package chattool_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/testutil"
)

func TestAnnotateInterception(t *testing.T) {
	t.Parallel()

	call := func(input string) fantasy.ToolCall {
		return fantasy.ToolCall{ID: "call-1", Name: "annotate_interception", Input: input}
	}

	t.Run("AnnotatesLatestInterception", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		now := dbtime.Now()
		older := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user.ID,
			StartedAt:   now.Add(-time.Hour),
			Annotations: database.AIBridgeInterceptionCapabilities([]string{"workspace"}),
		}, nil)
		latest := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user.ID,
			StartedAt:   now,
			Annotations: database.AIBridgeInterceptionCapabilities([]string{"workspace"}),
		}, nil)

		tool := chattool.AnnotateInterception(db, chattool.AnnotateInterceptionOptions{OwnerID: user.ID})
		resp, err := tool.Run(ctx, call(`{"linear_issue_ids":[" ENG-1234 "],"repo":"coder/coder","branch":"main"}`))
		require.NoError(t, err)

		var result struct {
			Annotated      bool   `json:"annotated"`
			InterceptionID string `json:"interception_id"`
		}
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.True(t, result.Annotated)
		require.Equal(t, latest.ID.String(), result.InterceptionID)
		// Server-derived annotations stay out of the model's view.
		require.NotContains(t, resp.Content, "capabilities")

		got, err := db.GetAIBridgeInterceptionByID(ctx, latest.ID)
		require.NoError(t, err)
		require.NotNil(t, got.Annotations.LinearIssueIDs)
		require.Equal(t, []string{"ENG-1234"}, *got.Annotations.LinearIssueIDs)
		require.Equal(t, "coder/coder", *got.Annotations.Repo)
		require.Equal(t, "main", *got.Annotations.Branch)
		// The update merges, so the server-derived key survives.
		require.NotNil(t, got.Annotations.Capabilities)
		require.Equal(t, []string{"workspace"}, *got.Annotations.Capabilities)

		untouched, err := db.GetAIBridgeInterceptionByID(ctx, older.ID)
		require.NoError(t, err)
		require.Nil(t, untouched.Annotations.Repo)
	})

	t.Run("OmittedFieldsKeepPreviousValues", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user.ID,
		}, nil)

		tool := chattool.AnnotateInterception(db, chattool.AnnotateInterceptionOptions{OwnerID: user.ID})
		_, err := tool.Run(ctx, call(`{"repo":"coder/coder","branch":"main"}`))
		require.NoError(t, err)
		_, err = tool.Run(ctx, call(`{"branch":"scott/feature"}`))
		require.NoError(t, err)

		got, err := db.GetAIBridgeInterceptionByID(ctx, interception.ID)
		require.NoError(t, err)
		require.Equal(t, "coder/coder", *got.Annotations.Repo)
		require.Equal(t, "scott/feature", *got.Annotations.Branch)
		require.Nil(t, got.Annotations.LinearIssueIDs)
	})

	t.Run("IssuesAccumulate", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user.ID,
		}, nil)

		tool := chattool.AnnotateInterception(db, chattool.AnnotateInterceptionOptions{OwnerID: user.ID})
		_, err := tool.Run(ctx, call(`{"linear_issue_ids":["PLAT-999"]}`))
		require.NoError(t, err)
		_, err = tool.Run(ctx, call(`{"linear_issue_ids":["PLAT-988","PLAT-999"]}`))
		require.NoError(t, err)

		got, err := db.GetAIBridgeInterceptionByID(ctx, interception.ID)
		require.NoError(t, err)
		require.NotNil(t, got.Annotations.LinearIssueIDs)
		require.Equal(t, []string{"PLAT-988", "PLAT-999"}, *got.Annotations.LinearIssueIDs)
	})

	t.Run("PullRequestsAccumulate", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user.ID,
		}, nil)

		tool := chattool.AnnotateInterception(db, chattool.AnnotateInterceptionOptions{OwnerID: user.ID})
		_, err := tool.Run(ctx, call(`{"github_pr_urls":["https://github.com/coder/coder/pull/28300"]}`))
		require.NoError(t, err)
		_, err = tool.Run(ctx, call(`{"github_pr_urls":["https://github.com/coder/coder/pull/28299"]}`))
		require.NoError(t, err)

		got, err := db.GetAIBridgeInterceptionByID(ctx, interception.ID)
		require.NoError(t, err)
		require.NotNil(t, got.Annotations.GitHubPRURLs)
		require.Equal(t, []string{
			"https://github.com/coder/coder/pull/28299",
			"https://github.com/coder/coder/pull/28300",
		}, *got.Annotations.GitHubPRURLs)
	})

	t.Run("RejectsNonPullRequestURLs", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user.ID,
		}, nil)
		tool := chattool.AnnotateInterception(db, chattool.AnnotateInterceptionOptions{OwnerID: user.ID})

		for _, url := range []string{
			"javascript:alert(1)",
			"http://github.com/coder/coder/pull/1",
			"https://github.example.com/coder/coder/pull/1",
			"https://github.com/coder/coder/issues/1",
			"https://github.com/coder/coder/pull/1?x=y",
		} {
			input, err := json.Marshal(map[string][]string{"github_pr_urls": {url}})
			require.NoError(t, err)
			resp, err := tool.Run(ctx, call(string(input)))
			require.NoError(t, err)
			require.Contains(t, resp.Content, "must look like https://github.com/", "url %q", url)
		}

		// A rejected call writes nothing.
		got, err := db.GetAIBridgeInterceptionByID(ctx, interception.ID)
		require.NoError(t, err)
		require.Nil(t, got.Annotations.GitHubPRURLs)
	})

	t.Run("NoFields", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		tool := chattool.AnnotateInterception(db, chattool.AnnotateInterceptionOptions{OwnerID: user.ID})

		resp, err := tool.Run(ctx, call(`{"repo":"   ","linear_issue_ids":[]}`))
		require.NoError(t, err)
		require.Contains(t, resp.Content, "provide at least one of")
	})

	t.Run("ValueTooLong", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		tool := chattool.AnnotateInterception(db, chattool.AnnotateInterceptionOptions{OwnerID: user.ID})

		input, err := json.Marshal(map[string]string{"repo": strings.Repeat("a", 257)})
		require.NoError(t, err)
		resp, err := tool.Run(ctx, call(string(input)))
		require.NoError(t, err)
		require.Contains(t, resp.Content, "256 characters or fewer")
	})

	t.Run("NoInterception", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		tool := chattool.AnnotateInterception(db, chattool.AnnotateInterceptionOptions{OwnerID: user.ID})

		resp, err := tool.Run(ctx, call(`{"repo":"coder/coder"}`))
		require.NoError(t, err)
		require.Contains(t, resp.Content, "no AI Bridge interception to annotate")
	})

	t.Run("OtherUsersInterceptionIsInvisible", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		other := dbgen.User(t, db, database.User{})
		_ = dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: other.ID,
		}, nil)

		tool := chattool.AnnotateInterception(db, chattool.AnnotateInterceptionOptions{OwnerID: user.ID})
		resp, err := tool.Run(ctx, call(`{"repo":"coder/coder"}`))
		require.NoError(t, err)
		require.Contains(t, resp.Content, "no AI Bridge interception to annotate")
	})
}
