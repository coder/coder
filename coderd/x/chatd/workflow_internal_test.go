package chatd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const reviewSchemaStarlark = `REVIEW = {"type": "object", "required": ["bugs"],
          "properties": {"bugs": {"type": "array", "items": {"type": "string"}}}}
`

func TestWorkflowReport(t *testing.T) {
	t.Parallel()

	schema := json.RawMessage(`{"type":"object","required":["bugs"],"properties":{"bugs":{"type":"array","items":{"type":"string"}}}}`)

	t.Run("ValidValueAccepted", func(t *testing.T) {
		t.Parallel()
		tool, err := newWorkflowReportTool(schema)
		require.NoError(t, err)
		assert.Equal(t, []string{"bugs"}, tool.Info().Required)

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"bugs":["nil map write"]}`})
		require.NoError(t, err)
		assert.False(t, resp.IsError, resp.Content)
	})

	t.Run("InvalidValueRejected", func(t *testing.T) {
		t.Parallel()
		tool, err := newWorkflowReportTool(schema)
		require.NoError(t, err)

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"bugs":"nil map write"}`})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "does not match the schema")
	})

	t.Run("SchemaChecks", func(t *testing.T) {
		t.Parallel()
		_, err := checkStepSchema(json.RawMessage(`{"type":"array"}`))
		require.ErrorContains(t, err, `"type": "object"`)
		_, err = checkStepSchema(json.RawMessage(`{"$schema":"http://json-schema.org/draft-04/schema#","type":"object"}`))
		require.ErrorContains(t, err, "unsupported $schema")
		_, err = checkStepSchema(json.RawMessage(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"object"}`))
		require.NoError(t, err)
	})

	t.Run("OnlyStepChildrenGetTool", func(t *testing.T) {
		t.Parallel()
		raw, err := workflowStepTools(schema)
		require.NoError(t, err)
		tools := pqtype.NullRawMessage{RawMessage: raw, Valid: true}

		root := database.Chat{ID: uuid.New(), DynamicTools: tools}
		got, err := appendWorkflowReportTool(nil, root)
		require.NoError(t, err)
		assert.Empty(t, got, "a root chat must not get workflow_report")

		child := root
		child.ParentChatID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
		got, err = appendWorkflowReportTool(nil, child)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, workflowReportToolName, got[0].Info().Name)
		assert.Contains(t, withWorkflowReportStop(nil, got), workflowReportToolName)
		assert.True(t, exploreToolAllowed(got[0]), "explore steps must keep workflow_report")

		// The client never runs workflow_report.
		names, err := parseDynamicToolNames(tools)
		require.NoError(t, err)
		assert.Empty(t, names)
		assert.Empty(t, dynamicToolNamesFromChat(child))
	})

	t.Run("StepIDs", func(t *testing.T) {
		t.Parallel()
		runID := uuid.New()
		assert.Equal(t, workflowStepID(runID, "k", 0), workflowStepID(runID, "k", 0))
		assert.NotEqual(t, workflowStepID(runID, "k", 0), workflowStepID(runID, "k", 1))
		assert.NotEqual(t, workflowStepID(runID, "k", 0), workflowStepID(uuid.New(), "k", 0))
		callID := uuid.New()
		assert.Equal(t, workflowRunID(callID), workflowRunID(callID))
	})
}

// workflowFixture drives runs on a passive server. The test plays each
// step child: it writes the child's messages and status by hand.
type workflowFixture struct {
	db     database.Store
	ps     pubsub.Pubsub
	server *Server
	user   database.User
	org    database.Organization
	model  database.ChatModelConfig
}

func newWorkflowFixture(t *testing.T) *workflowFixture {
	t.Helper()
	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	user, org, model := seedInternalChatDeps(t, db)
	return &workflowFixture{db: db, ps: ps, server: server, user: user, org: org, model: model}
}

func (f *workflowFixture) parent(ctx context.Context, t *testing.T, status database.ChatStatus) database.Chat {
	t.Helper()
	parent, err := f.server.CreateChat(ctx, CreateOptions{
		OrganizationID:     f.org.ID,
		OwnerID:            f.user.ID,
		Title:              "parent-" + t.Name(),
		ModelConfigID:      f.model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)
	setChatStatus(ctx, t, f.db, parent.ID, status, "")
	parent, err = f.db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)
	return parent
}

func (f *workflowFixture) start(
	ctx context.Context, t *testing.T, parent database.Chat, args runWorkflowArgs,
) *workflowRun {
	t.Helper()
	run, err := f.server.startWorkflowRun(ctx, parent, f.model.ID, args, uuid.New())
	require.NoError(t, err)
	return run
}

// steps waits until the run has n steps.
func (*workflowFixture) steps(ctx context.Context, t *testing.T, run *workflowRun, n int) []stepSummary {
	t.Helper()
	var steps []stepSummary
	testutil.Eventually(ctx, t, func(context.Context) bool {
		steps = run.snapshot().Steps
		return len(steps) >= n
	}, testutil.IntervalFast)
	return steps
}

func (f *workflowFixture) insert(t *testing.T, chatID uuid.UUID, role database.ChatMessageRole, parts ...codersdk.ChatMessagePart) {
	t.Helper()
	data, err := json.Marshal(parts)
	require.NoError(t, err)
	_ = dbgen.ChatMessage(t, f.db, database.ChatMessage{
		ChatID:         chatID,
		ModelConfigID:  uuid.NullUUID{UUID: f.model.ID, Valid: true},
		Role:           role,
		Content:        pqtype.NullRawMessage{RawMessage: data, Valid: true},
		ContentVersion: chatprompt.ContentVersionV1,
	})
}

// endTurn sets the child to waiting and wakes awaitSubagentCompletion.
func (f *workflowFixture) endTurn(ctx context.Context, t *testing.T, chatID uuid.UUID) {
	t.Helper()
	setChatStatus(ctx, t, f.db, chatID, database.ChatStatusWaiting, "")
	require.NoError(t, f.ps.Publish(coderdpubsub.ChatStateUpdateChannel(chatID), []byte("{}")))
}

// report plays a child that calls workflow_report with value.
func (f *workflowFixture) report(ctx context.Context, t *testing.T, chatID string, value string) {
	t.Helper()
	id := uuid.MustParse(chatID)
	callID := "call-" + uuid.NewString()
	f.insert(t, id, database.ChatMessageRoleAssistant,
		codersdk.ChatMessageToolCall(callID, workflowReportToolName, json.RawMessage(value)))
	f.insert(t, id, database.ChatMessageRoleTool,
		codersdk.ChatMessageToolResult(callID, workflowReportToolName, json.RawMessage(`"Value accepted."`), false, false))
	f.endTurn(ctx, t, id)
}

// stopWithoutReport plays a child that ends its turn with text only.
func (f *workflowFixture) stopWithoutReport(ctx context.Context, t *testing.T, chatID string) {
	t.Helper()
	id := uuid.MustParse(chatID)
	f.insert(t, id, database.ChatMessageRoleAssistant, codersdk.ChatMessageText("I am done."))
	f.endTurn(ctx, t, id)
}

func (f *workflowFixture) userTexts(ctx context.Context, t *testing.T, chatID uuid.UUID) []string {
	t.Helper()
	msgs, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chatID})
	require.NoError(t, err)
	var texts []string
	for _, msg := range msgs {
		if msg.Role != database.ChatMessageRoleUser {
			continue
		}
		parts, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		for _, part := range parts {
			if part.Type == codersdk.ChatMessagePartTypeText {
				texts = append(texts, part.Text)
			}
		}
	}
	return texts
}

func (f *workflowFixture) hasNotice(ctx context.Context, t *testing.T, chatID uuid.UUID) bool {
	t.Helper()
	for _, text := range f.userTexts(ctx, t, chatID) {
		if strings.Contains(text, "ended with status") {
			return true
		}
	}
	return false
}

// awaitRegistered starts await in a goroutine and returns once the run
// counts it, so finish sees a waiting call.
func awaitRegistered(ctx context.Context, t *testing.T, run *workflowRun) <-chan runWorkflowResult {
	t.Helper()
	out := make(chan runWorkflowResult, 1)
	go func() { out <- run.await(ctx, testutil.WaitLong) }()
	testutil.Eventually(ctx, t, func(context.Context) bool {
		run.mu.Lock()
		defer run.mu.Unlock()
		return run.awaiters == 1
	}, testutil.IntervalFast)
	return out
}

func TestWorkflowReplay(t *testing.T) {
	t.Parallel()
	f := newWorkflowFixture(t)
	ctx := chatdTestContext(t)
	parent := f.parent(ctx, t, database.ChatStatusRunning)

	args := runWorkflowArgs{
		Script: reviewSchemaStarlark + `
def main(args):
    steps = [spawn("Review %s for bugs." % name, schema = REVIEW, title = name) for name in args["files"]]
    reviews = wait(steps)
    bugs = [b for r in reviews if r != None for b in r["bugs"]]
    log("%d bugs" % len(bugs))
    return {"bugs": bugs}
`,
		Args: map[string]any{"files": []string{"a.go", "b.go"}},
	}
	run := f.start(ctx, t, parent, args)
	assert.Equal(t, workflowStatusRunning, run.snapshot().Status, "run_workflow returns at once")
	done := awaitRegistered(ctx, t, run)

	steps := f.steps(ctx, t, run, 2)
	f.report(ctx, t, steps[0].ChatID, `{"bugs":["nil map write"]}`)
	f.report(ctx, t, steps[1].ChatID, `{"bugs":[]}`)

	res := testutil.RequireReceive(ctx, t, done)
	require.Equal(t, workflowStatusCompleted, res.Status, res.Error)
	assert.JSONEq(t, `{"bugs":["nil map write"]}`, string(res.Result))
	assert.Equal(t, []string{"1 bugs"}, res.Logs)
	assert.False(t, f.hasNotice(ctx, t, parent.ID), "the waiting call got the result, so no message")

	// A restart loses the run. The model resumes it with the same script.
	f.server.workflows.remove(run.id)
	args.ResumeRunID = run.id.String()
	resumed := f.start(ctx, t, parent, args)
	res = resumed.await(ctx, testutil.WaitLong)
	require.Equal(t, workflowStatusCompleted, res.Status, res.Error)
	assert.JSONEq(t, `{"bugs":["nil map write"]}`, string(res.Result))
	require.Len(t, res.Steps, 2)
	for i, step := range res.Steps {
		assert.True(t, step.Reused, "step %d must be reused", i)
		assert.Equal(t, steps[i].ChatID, step.ChatID)
	}
}

func TestWorkflowRemind(t *testing.T) {
	t.Parallel()
	f := newWorkflowFixture(t)

	t.Run("ReportAfterReminder", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		parent := f.parent(ctx, t, database.ChatStatusRunning)
		run := f.start(ctx, t, parent, runWorkflowArgs{Script: `
def main(args):
    return agent("Say hi.", title = "hi")
`})
		step := f.steps(ctx, t, run, 1)[0]
		childID := uuid.MustParse(step.ChatID)
		f.stopWithoutReport(ctx, t, step.ChatID)
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			return strings.Contains(strings.Join(f.userTexts(ctx, t, childID), "\n"), workflowReminder)
		}, testutil.IntervalFast)
		f.report(ctx, t, step.ChatID, `{"text":"hi"}`)

		res := run.await(ctx, testutil.WaitLong)
		require.Equal(t, workflowStatusCompleted, res.Status, res.Error)
		assert.JSONEq(t, `{"text":"hi"}`, string(res.Result))
	})

	t.Run("NoReportGivesNone", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		parent := f.parent(ctx, t, database.ChatStatusRunning)
		run := f.start(ctx, t, parent, runWorkflowArgs{Script: `
def main(args):
    return wait([spawn("Say hi.")])
`})
		step := f.steps(ctx, t, run, 1)[0]
		childID := uuid.MustParse(step.ChatID)
		f.stopWithoutReport(ctx, t, step.ChatID)
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			return strings.Contains(strings.Join(f.userTexts(ctx, t, childID), "\n"), workflowReminder)
		}, testutil.IntervalFast)
		f.stopWithoutReport(ctx, t, step.ChatID)

		res := run.await(ctx, testutil.WaitLong)
		require.Equal(t, workflowStatusCompleted, res.Status, res.Error)
		assert.JSONEq(t, `[null]`, string(res.Result))
		require.Len(t, res.Steps, 1)
		assert.Equal(t, stepStatusFailed, res.Steps[0].Status)
	})
}

func TestWorkflowNotice(t *testing.T) {
	t.Parallel()
	f := newWorkflowFixture(t)
	script := runWorkflowArgs{Script: `
def main(args):
    return agent("Say hi.")
`}

	t.Run("IdleParentGetsMessage", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		// The parent turn ended after run_workflow returned.
		parent := f.parent(ctx, t, database.ChatStatusWaiting)
		run := f.start(ctx, t, parent, script)
		f.report(ctx, t, f.steps(ctx, t, run, 1)[0].ChatID, `{"text":"hi"}`)

		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			return f.hasNotice(ctx, t, parent.ID)
		}, testutil.IntervalFast)
		res := run.await(ctx, 0)
		require.Equal(t, workflowStatusCompleted, res.Status, res.Error)
	})

	t.Run("AwaitDeletesQueuedMessage", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		// The parent is busy, so the message waits in the queue.
		parent := f.parent(ctx, t, database.ChatStatusRunning)
		run := f.start(ctx, t, parent, script)
		f.report(ctx, t, f.steps(ctx, t, run, 1)[0].ChatID, `{"text":"hi"}`)

		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			n, err := f.db.CountChatQueuedMessages(ctx, parent.ID)
			return err == nil && n == 1
		}, testutil.IntervalFast)
		res := run.await(ctx, 0)
		require.Equal(t, workflowStatusCompleted, res.Status, res.Error)
		n, err := f.db.CountChatQueuedMessages(ctx, parent.ID)
		require.NoError(t, err)
		assert.Zero(t, n, "await_workflow read the result, so the message goes")
	})
}

func TestWorkflowLimit(t *testing.T) {
	t.Parallel()
	f := newWorkflowFixture(t)

	t.Run("Steps", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		parent := f.parent(ctx, t, database.ChatStatusRunning)
		run := f.start(ctx, t, parent, runWorkflowArgs{Script: `
def main(args):
    for i in range(51):
        spawn("Step %d." % i)
    return {}
`})
		res := run.await(ctx, testutil.WaitLong)
		require.Equal(t, workflowStatusFailed, res.Status)
		assert.Contains(t, res.Error, "can create 50 steps at most")
		assert.Len(t, res.Steps, maxWorkflowSteps)
	})

	t.Run("CompileErrors", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		parent := f.parent(ctx, t, database.ChatStatusRunning)
		_, err := f.server.startWorkflowRun(ctx, parent, f.model.ID, runWorkflowArgs{Script: "def main(args):\n  return ("}, uuid.New())
		require.ErrorContains(t, err, "compile script")
		_, err = f.server.startWorkflowRun(ctx, parent, f.model.ID, runWorkflowArgs{Script: "x = 1\n"}, uuid.New())
		require.ErrorContains(t, err, "must define main(args)")
		_, err = f.server.startWorkflowRun(ctx, parent, f.model.ID, runWorkflowArgs{Script: "def main(args):\n  return time.now()\n"}, uuid.New())
		require.ErrorContains(t, err, "undefined: time")
	})

	t.Run("RuntimeErrorNamesLine", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		parent := f.parent(ctx, t, database.ChatStatusRunning)
		run := f.start(ctx, t, parent, runWorkflowArgs{Script: "def main(args):\n  fail(\"boom\")\n"})
		res := run.await(ctx, testutil.WaitLong)
		require.Equal(t, workflowStatusFailed, res.Status)
		assert.Contains(t, res.Error, "boom")
		assert.Contains(t, res.Error, "workflow.star:2")
	})

	t.Run("AwaitTimeoutReturnsRunning", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		parent := f.parent(ctx, t, database.ChatStatusRunning)
		run := f.start(ctx, t, parent, runWorkflowArgs{Script: "def main(args):\n  return agent(\"Wait.\")\n"})
		f.steps(ctx, t, run, 1)
		res := run.await(ctx, 0)
		assert.Equal(t, workflowStatusRunning, res.Status)
		assert.Len(t, res.Steps, 1)
	})
}
