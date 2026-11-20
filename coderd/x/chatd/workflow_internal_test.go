package chatd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
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
	program, err := compileWorkflow(args.ScriptPath, args.Script)
	require.NoError(t, err)
	run, err := f.server.startWorkflowRun(ctx, parent, f.model.ID, program, args.Args, args.ResumeRunID, uuid.New())
	require.NoError(t, err)
	return run
}

// startFlow starts an inline flow the way run_flow does.
func (f *workflowFixture) startFlow(
	ctx context.Context, t *testing.T, parent database.Chat, args runFlowArgs,
) *workflowRun {
	t.Helper()
	src, err := flowSource(ctx, args, nil)
	require.NoError(t, err)
	program, err := compileFlow(src)
	require.NoError(t, err)
	run, err := f.server.startWorkflowRun(ctx, parent, f.model.ID, program, args.Args, args.ResumeRunID, uuid.New())
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

func TestWorkflowScriptPath(t *testing.T) {
	t.Parallel()
	const path = "/home/coder/workflow.star"
	connWithFile := func(t *testing.T, content string) func(context.Context) (workspacesdk.AgentConn, error) {
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		// The limit is one byte over the cap, so an oversized file is caught.
		conn.EXPECT().ReadFile(gomock.Any(), path, int64(0), int64(maxScriptBytes+1)).
			Return(io.NopCloser(strings.NewReader(content)), "", nil)
		return func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil }
	}
	noConn := func(context.Context) (workspacesdk.AgentConn, error) {
		return nil, xerrors.New("the workspace must not be read")
	}

	t.Run("ReadsTheFile", func(t *testing.T) {
		t.Parallel()
		const src = "def main(args):\n    return 1\n"
		got, err := workflowScript(context.Background(), runWorkflowArgs{ScriptPath: path}, connWithFile(t, src))
		require.NoError(t, err)
		assert.Equal(t, src, got)
	})
	t.Run("InlineScriptSkipsTheWorkspace", func(t *testing.T) {
		t.Parallel()
		got, err := workflowScript(context.Background(), runWorkflowArgs{Script: "x"}, noConn)
		require.NoError(t, err)
		assert.Equal(t, "x", got)
	})
	t.Run("RejectsBoth", func(t *testing.T) {
		t.Parallel()
		_, err := workflowScript(context.Background(), runWorkflowArgs{Script: "x", ScriptPath: path}, noConn)
		require.ErrorContains(t, err, "not both")
	})
	t.Run("RejectsAnOversizedFile", func(t *testing.T) {
		t.Parallel()
		big := strings.Repeat("#", maxScriptBytes+1)
		_, err := workflowScript(context.Background(), runWorkflowArgs{ScriptPath: path}, connWithFile(t, big))
		require.ErrorContains(t, err, "larger than")
	})
	t.Run("MissingFileHidesTheAgentURL", func(t *testing.T) {
		t.Parallel()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		sdkErr := codersdk.NewTestError(404, "GET", "http://127.0.0.1:4/api/v0/read-file?path=x")
		sdkErr.Message = "open " + path + ": no such file or directory"
		conn.EXPECT().ReadFile(gomock.Any(), path, int64(0), int64(maxScriptBytes+1)).Return(nil, "", sdkErr).Times(3)
		getConn := func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil }

		_, err := workflowScript(context.Background(), runWorkflowArgs{ScriptPath: path}, getConn)
		require.EqualError(t, err, `read script_path "/home/coder/workflow.star": open /home/coder/workflow.star: no such file or directory`)
		assert.NotContains(t, err.Error(), "http")

		sdkErr.Detail = "permission denied"
		_, err = workflowScript(context.Background(), runWorkflowArgs{ScriptPath: path}, getConn)
		require.EqualError(t, err, `read script_path "/home/coder/workflow.star": open /home/coder/workflow.star: no such file or directory: permission denied`)

		// run_flow reads flow_path with the same code.
		_, err = flowSource(context.Background(), runFlowArgs{FlowPath: path}, getConn)
		require.EqualError(t, err, `read flow_path "/home/coder/workflow.star": open /home/coder/workflow.star: no such file or directory: permission denied`)
	})
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
		// await_workflow read the result, so the message goes. The sender can
		// still hold the message ID, and then it deletes the message itself.
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			n, err := f.db.CountChatQueuedMessages(ctx, parent.ID)
			return err == nil && n == 0
		}, testutil.IntervalFast)
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
		_, err := compileWorkflow("", "def main(args):\n  return (")
		require.ErrorContains(t, err, "compile script: workflow.star:2:")
		_, err = compileWorkflow("", "x = 1\n")
		require.ErrorContains(t, err, "must define main(args)")
		_, err = compileWorkflow("", "def main(args):\n  return time.now()\n")
		require.ErrorContains(t, err, "undefined: time")
		// A script from a file names the file, not workflow.star.
		_, err = compileWorkflow("/home/coder/review.star", "def main(args):\n  return (")
		require.ErrorContains(t, err, "compile script: /home/coder/review.star:2:")
		assert.NotContains(t, err.Error(), workflowFileName)
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

func TestFlowCheck(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		flow string
		want []string // one problem per line, in sorted order
	}{
		{
			name: "UnknownFieldsAndTypes",
			flow: `{"start": "a", "outputs": "1",
				"nodes": {"a": {"prompt": "x", "foreach": "input.files", "max_visits": "2"}},
				"edges": [{"from": "a", "to": "a", "next": "b"}]}`,
			want: []string{
				`edges[0]: unknown field "next"`,
				`flow: unknown field "outputs"`,
				`nodes.a.max_visits: got a JSON string, want an integer`,
				`nodes.a: unknown field "foreach"`,
			},
		},
		{
			name: "Structure",
			flow: `{"start": "missing",
				"nodes": {
					"Bad": {"prompt": "x"},
					"a": {"prompt": " ", "type": "planner", "schema": {"type": "array"}, "max_visits": 51},
					"b": {"prompt": "x"}},
				"edges": [{"from": "a", "to": "ghost"}, {"to": "b"}]}`,
			want: []string{
				`edges[0].to: "ghost" is not a node`,
				`edges[1].from: required`,
				`nodes.Bad: a node ID must match ^[a-z][a-z0-9_]*$`,
				`nodes.a.max_visits: must be from 1 to 50`,
				`nodes.a.prompt: required`,
				`nodes.a.schema: the step schema must have "type": "object"`,
				`nodes.a.type: must be "general" or "explore"`,
				`nodes: the max_visits of the nodes without for_each add up to 53`,
				`start: "missing" is not a node`,
			},
		},
		{
			name: "CEL",
			flow: `{"start": "a", "output": "nodes.a.valu",
				"nodes": {
					"a": {"prompt": "Use {{ item }} and {{ nodes.ghost.value }}.", "for_each": "'one'"},
					"b": {"prompt": "x {{ input.x", "title": "{{ input.( }}"},
					"c": {"prompt": "Only a for_each node has {{ item }}."}},
				"edges": [{"from": "a", "to": "b", "when": "1"}, {"from": "a", "to": "c", "when": "valu.ok"}]}`,
			want: []string{
				`edges[0].when: must return bool, got int`,
				`edges[1].when: 1:1: undeclared reference to 'valu'`,
				`nodes.a.for_each: must return a list, got string`,
				`nodes.a.prompt {{ nodes.ghost.value }}: nodes.ghost is not a node`,
				`nodes.b.prompt: a {{ has no closing }}`,
				`nodes.b.title {{ input.( }}: 1:7: Syntax error`,
				`nodes.c.prompt {{ item }}: 1:1: undeclared reference to 'item'`,
				`output: nodes.a.valu: a node has only value and values`,
			},
		},
		{
			name: "Unreachable",
			flow: `{"start": "a", "nodes": {"a": {"prompt": "x"}, "b": {"prompt": "y"}}, "edges": []}`,
			want: []string{`nodes.b: not reachable from start "a"`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := compileFlow([]byte(tc.flow))
			require.Error(t, err)
			lines := strings.Split(err.Error(), "\n")
			require.Equal(t, "invalid flow:", lines[0])
			require.Len(t, lines[1:], len(tc.want), err.Error())
			for i, want := range tc.want {
				assert.Contains(t, lines[i+1], want)
			}
		})
	}

	t.Run("NotJSON", func(t *testing.T) {
		t.Parallel()
		_, err := compileFlow([]byte(`{"start": `))
		require.ErrorContains(t, err, "not valid JSON")
	})
}

func TestFlowTemplate(t *testing.T) {
	t.Parallel()
	program, err := compileFlow([]byte(`{"start": "a", "nodes": {"a": {"prompt": ` +
		`"s={{ input.name }} n={{ input.n }} m={{ input.files.transformMapEntry(i, f, {f: i}) }} ` +
		`l={{ input.files.map(f, {'f': f, 'len': f.size()}) }} lit={{ {'k': {'v': null}} }} b={{ \"{{\" }}"}}}`))
	require.NoError(t, err)
	state := &flowState{
		input:  map[string]any{"name": "a<b", "n": 3.0, "files": []any{"a.go", "bb.go"}},
		values: map[string][]any{"a": {}},
	}
	got, err := program.nodes["a"].prompt.render(context.Background(), state.vars())
	require.NoError(t, err)
	assert.Equal(t, `s=a<b n=3 m={"a.go":0,"bb.go":1} l=[{"f":"a.go","len":4},{"f":"bb.go","len":5}] lit={"k":{"v":null}} b={{`, got)
}

func TestFlowForEach(t *testing.T) {
	t.Parallel()
	f := newWorkflowFixture(t)
	ctx := chatdTestContext(t)
	parent := f.parent(ctx, t, database.ChatStatusRunning)

	// review fans out one step per file, then merge reads all reviews.
	flow := runFlowArgs{
		Flow: map[string]any{
			"start": "review",
			"nodes": map[string]any{
				"review": map[string]any{
					"for_each": "input.files",
					"prompt":   "Review {{ item }} ({{ index }}).",
					"schema":   json.RawMessage(`{"type":"object","required":["bugs"],"properties":{"bugs":{"type":"array","items":{"type":"string"}}}}`),
				},
				"merge": map[string]any{"prompt": "Merge {{ nodes.review.value }}."},
			},
			"edges": []any{map[string]any{"from": "review", "to": "merge"}},
			"output": "{'bugs': nodes.review.value.filter(r, r != null).map(r, r.bugs).flatten(), " +
				"'failed': nodes.review.value.filter(r, r == null).size(), 'summary': nodes.merge.value.text}",
		},
		Args: map[string]any{"files": []string{"a.go", "b.go", "c.go"}},
	}

	run := f.startFlow(ctx, t, parent, flow)
	steps := f.steps(ctx, t, run, 3)
	titles := []string{steps[0].Title, steps[1].Title, steps[2].Title}
	assert.Equal(t, []string{"review [0]", "review [1]", "review [2]"}, titles)
	assert.Contains(t, f.userTexts(ctx, t, uuid.MustParse(steps[1].ChatID)), "Review b.go (1).")

	// The steps finish out of order. b.go stops twice without a report,
	// so it fails. The run waits for the items in order, so b.go gets its
	// reminder after a.go is done.
	f.report(ctx, t, steps[2].ChatID, `{"bugs":["y"]}`)
	f.report(ctx, t, steps[0].ChatID, `{"bugs":["x"]}`)
	f.stopWithoutReport(ctx, t, steps[1].ChatID)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return strings.Contains(strings.Join(f.userTexts(ctx, t, uuid.MustParse(steps[1].ChatID)), "\n"), workflowReminder)
	}, testutil.IntervalFast)
	f.stopWithoutReport(ctx, t, steps[1].ChatID)

	merge := f.steps(ctx, t, run, 4)[3]
	assert.Equal(t, "merge", merge.Title)
	assert.Contains(t, f.userTexts(ctx, t, uuid.MustParse(merge.ChatID)), `Merge [{"bugs":["x"]},null,{"bugs":["y"]}].`)
	f.report(ctx, t, merge.ChatID, `{"text":"two bugs"}`)

	res := run.await(ctx, testutil.WaitLong)
	require.Equal(t, workflowStatusCompleted, res.Status, res.Error)
	assert.JSONEq(t, `{"bugs":["x","y"],"failed":1,"summary":"two bugs"}`, string(res.Result))
}

func TestFlowLoop(t *testing.T) {
	t.Parallel()
	f := newWorkflowFixture(t)
	// draft and critique alternate while the score is below 8.
	loop := func(maxVisits int) runFlowArgs {
		return runFlowArgs{Flow: map[string]any{
			"start": "draft",
			"nodes": map[string]any{
				"draft": map[string]any{"prompt": "Draft {{ nodes.draft.values.size() + 1 }}.", "max_visits": maxVisits},
				"critique": map[string]any{
					"prompt":     "Score {{ nodes.draft.value.text }}.",
					"schema":     json.RawMessage(`{"type":"object","required":["score"],"properties":{"score":{"type":"number"}}}`),
					"max_visits": maxVisits,
				},
			},
			"edges": []any{
				map[string]any{"from": "draft", "to": "critique"},
				map[string]any{"from": "critique", "to": "draft", "when": "value.score < 8"},
			},
			"output": "nodes.critique.values.map(c, c.score)",
		}}
	}
	// play reports each step in turn: drafts get text, critiques a score.
	play := func(ctx context.Context, t *testing.T, run *workflowRun, scores ...int) []stepSummary {
		var steps []stepSummary
		for i, score := range scores {
			steps = f.steps(ctx, t, run, 2*i+1)
			f.report(ctx, t, steps[2*i].ChatID, fmt.Sprintf(`{"text":"v%d"}`, i+1))
			steps = f.steps(ctx, t, run, 2*i+2)
			f.report(ctx, t, steps[2*i+1].ChatID, fmt.Sprintf(`{"score":%d}`, score))
		}
		return steps
	}

	t.Run("BackEdgeUntilWhenIsFalse", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		parent := f.parent(ctx, t, database.ChatStatusRunning)
		args := loop(3)
		run := f.startFlow(ctx, t, parent, args)
		steps := play(ctx, t, run, 5, 9)

		res := run.await(ctx, testutil.WaitLong)
		require.Equal(t, workflowStatusCompleted, res.Status, res.Error)
		assert.JSONEq(t, `[5, 9]`, string(res.Result))
		var titles []string
		for _, step := range res.Steps {
			titles = append(titles, step.Title)
		}
		assert.Equal(t, []string{"draft", "critique", "draft #2", "critique #2"}, titles)
		assert.Contains(t, f.userTexts(ctx, t, uuid.MustParse(steps[3].ChatID)), "Score v2.")
		assert.Contains(t, res.Logs, "critique visit 2 done, no edge matched, the flow ends")

		// A restart loses the run. The model resumes it with the same flow,
		// and the reused values take the same path.
		f.server.workflows.remove(run.id)
		args.ResumeRunID = run.id.String()
		resumed := f.startFlow(ctx, t, parent, args)
		res = resumed.await(ctx, testutil.WaitLong)
		require.Equal(t, workflowStatusCompleted, res.Status, res.Error)
		assert.JSONEq(t, `[5, 9]`, string(res.Result))
		require.Len(t, res.Steps, 4)
		for i, step := range res.Steps {
			assert.True(t, step.Reused, "step %d must be reused", i)
			assert.Equal(t, steps[i].ChatID, step.ChatID)
		}
	})

	t.Run("MaxVisitsFailsTheRun", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		parent := f.parent(ctx, t, database.ChatStatusRunning)
		run := f.startFlow(ctx, t, parent, loop(2))
		play(ctx, t, run, 5, 5)

		res := run.await(ctx, testutil.WaitLong)
		require.Equal(t, workflowStatusFailed, res.Status)
		assert.Equal(t, "node draft ran its max_visits (2) times", res.Error)
	})
}

// The examples in the run_flow description must pass the checks, since a
// model copies them.
func TestFlowDescriptionExamples(t *testing.T) {
	t.Parallel()
	start := strings.Index(runFlowDescription, `{"start": "review"`)
	end := strings.Index(runFlowDescription, "\n\nLoop example")
	require.True(t, start >= 0 && end > start)
	_, err := compileFlow([]byte(runFlowDescription[start:end]))
	require.NoError(t, err)

	start = strings.Index(runFlowDescription, `"edges": [{"from": "draft"`)
	end = strings.Index(runFlowDescription, "\n\nResume:")
	require.True(t, start >= 0 && end > start)
	_, err = compileFlow([]byte(`{"start": "draft", "nodes": {
		"draft": {"prompt": "Write it. Feedback: {{ nodes.critique.values }}", "max_visits": 3},
		"critique": {"prompt": "Score {{ nodes.draft.value.text }}.", "max_visits": 3,
			"schema": {"type": "object", "required": ["score"], "properties": {"score": {"type": "number"}}}}},
		` + runFlowDescription[start:end] + `}`))
	require.NoError(t, err)
}
