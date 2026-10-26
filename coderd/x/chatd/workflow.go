package chatd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	runWorkflowToolName   = "run_workflow"
	awaitWorkflowToolName = "await_workflow"

	maxWorkflowSteps    = 50         // child chats per run, reused ones included
	maxScriptBytes      = 64 << 10   // the script in the tool input
	maxScriptSteps      = 10_000_000 // Starlark execution steps per run
	maxResultBytes      = 64 << 10   // the JSON that main(args) returns
	maxWorkflowLogs     = 50         // log() lines that a result keeps
	defaultWorkflowWait = 600 * time.Second
	// maxWorkflowWait leaves 2 minutes below defaultTaskTimeout, because
	// turn setup uses the same task attempt.
	maxWorkflowWait = 780 * time.Second
	// workflowStepTimeout bounds one step. A step that runs longer fails.
	workflowStepTimeout = time.Hour

	workflowStatusRunning   = "running"
	workflowStatusCompleted = "completed"
	workflowStatusFailed    = "failed"

	stepStatusRunning  = "running"
	stepStatusReminded = "reminded"
	stepStatusReported = "reported"
	stepStatusFailed   = "failed"
)

// defaultStepSchema applies when a step has no schema.
const defaultStepSchema = `{"type":"object","properties":{"text":{"type":"string","description":"Your answer."}},"required":["text"]}`

// workflowStepPrompt goes into the system prompt of each step child.
const workflowStepPrompt = "You are one step of a workflow. When your work is done, call the " +
	workflowReportToolName + " tool one time with your result. The workflow reads only that value."

const runWorkflowDescription = `Start a workflow run: a Starlark script (a Python dialect) that runs sub-agent steps and combines their results. ` +
	`The tool returns at once with a run_id and status "running". The run continues in the background. ` +
	`Call await_workflow with the run_id to get the result. If your turn ends first, you get a message when the run ends. Then call await_workflow.

Send the script inline in "script", or write it to a file in the workspace and send its absolute path in "script_path". Send one of the two. ` +
	`chatd reads the file one time, when the run starts. A later edit of the file does not change a run that has started. ` +
	`With a file, you can fix a script with small edits and start it again without sending the whole script. ` +
	`run_workflow runs after the other tool calls in the same batch, so a write or edit of the file in that batch is done first.

The script must define main(args). args is the "args" input (a JSON object). main must return a JSON value (dict, list, string, number, bool or None).

Built-ins:
- agent(prompt, type="general", schema=None, title="") starts one step, waits for it, and returns its value. A failed step fails the script.
- spawn(prompt, type="general", schema=None, title="") starts one step and returns a handle at once.
- wait(handles) waits for one handle or a list of handles. It returns the value, or a list of values in input order. A failed step gives None.
- log(message) adds a line to the run log. print() does the same.
- json.encode(value) and json.decode(text).

type is "general" or "explore". schema is a JSON Schema dict with "type": "object" at the top level. ` +
	`The step must call its workflow_report tool with a value that matches the schema. That value is the step value. Without a schema the value is {"text": "..."}.

Rules: the script has no load(), no clock, no file access and no network. while loops are allowed. Recursion is not allowed. ` +
	`Limits: script 64 KiB, 50 steps per run, 10,000,000 execution steps, result 64 KiB. ` +
	`Each step is a child chat of this chat. chatd limits how many sub-agent chats run at the same time. Extra steps wait in a queue.

Resume: a run lives in the memory of one coderd replica. If await_workflow says that the run is not found, for example after a restart, ` +
	`call run_workflow again with the same script (or script_path), the same args and resume_run_id. Finished steps are reused and do not run again.`

const awaitWorkflowDescription = `Wait for a workflow run that run_workflow started. ` +
	`It returns when the run ends, or after wait_seconds (default 600, at most 780). ` +
	`After a timeout the status is "running": call it again. ` +
	`The result has status (running, completed or failed), result (what main returned), error, steps, and the last 50 log lines.`

const workflowNotFoundHelp = `The run is not found on this coderd replica, for example because coderd restarted. ` +
	`Call run_workflow again with the same script (or script_path), the same args and resume_run_id set to this run_id. Finished steps are reused.`

type runWorkflowArgs struct {
	Script      string `json:"script,omitempty" description:"Starlark source that defines main(args). Send this or script_path."`
	ScriptPath  string `json:"script_path,omitempty" description:"Absolute path of a file in the workspace that holds the Starlark source. Send this or script."`
	Args        any    `json:"args,omitempty" description:"A JSON object. main(args) receives it."`
	ResumeRunID string `json:"resume_run_id,omitempty" description:"Continue this run after a restart. Send the same script (or script_path) and args again."`
}

type awaitWorkflowArgs struct {
	RunID       string `json:"run_id"`
	WaitSeconds *int   `json:"wait_seconds,omitempty" description:"Defaults to 600. At most 780. 0 returns the status at once."`
}

func (a awaitWorkflowArgs) wait() time.Duration {
	if a.WaitSeconds == nil {
		return defaultWorkflowWait
	}
	d := time.Duration(*a.WaitSeconds) * time.Second
	return min(max(d, 0), maxWorkflowWait)
}

// runWorkflowResult is the output of both tools.
type runWorkflowResult struct {
	RunID  string          `json:"run_id"`
	Status string          `json:"status"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
	Steps  []stepSummary   `json:"steps"`
	Logs   []string        `json:"logs,omitempty"`
}

type stepSummary struct {
	ChatID string `json:"chat_id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Reused bool   `json:"reused,omitempty"`
	Error  string `json:"error,omitempty"`
}

// workflowTools returns run_workflow and await_workflow for a root chat
// turn that is not in plan mode. chat-workflows is user-scoped, so an admin
// can turn it on at runtime with an experiment rule. The prototype decides
// on each step, not once for each turn.
func (p *Server) workflowTools(ctx context.Context, opts rootChatToolsOptions) []fantasy.AgentTool {
	if opts.isPlanModeTurn || opts.chat.ParentChatID.Valid || isExploreSubagentMode(opts.chat.Mode) ||
		!p.experimentEvaluator.Enabled(ctx, opts.chat.OwnerID, codersdk.ExperimentChatWorkflows) {
		return nil
	}
	return []fantasy.AgentTool{
		serialWorkflowTool{fantasy.NewAgentTool(
			runWorkflowToolName,
			runWorkflowDescription,
			func(ctx context.Context, args runWorkflowArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				// chatd sets this ID for each call. It is stable on every retry.
				callID, ok := workspacesdk.ToolCallIDFromContext(ctx)
				if !ok {
					return fantasy.NewTextErrorResponse("run_workflow needs a tool call ID"), nil
				}
				script, err := workflowScript(ctx, args, opts.workspaceCtx.getWorkspaceConn)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				args.Script = script
				run, err := p.startWorkflowRun(ctx, opts.chat, opts.modelConfigID, args, callID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				return workflowJSON(run.snapshot()), nil
			},
		)},
		fantasy.NewAgentTool(
			awaitWorkflowToolName,
			awaitWorkflowDescription,
			func(ctx context.Context, args awaitWorkflowArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				runID, err := uuid.Parse(strings.TrimSpace(args.RunID))
				if err != nil {
					return fantasy.NewTextErrorResponse("run_id must be a valid UUID"), nil
				}
				run := p.workflows.get(runID)
				if run == nil || run.parent.ID != opts.chat.ID {
					return fantasy.NewTextErrorResponse(workflowNotFoundHelp), nil
				}
				return workflowJSON(run.await(ctx, args.wait())), nil
			},
		),
	}
}

// serialWorkflowTool makes chatloop run run_workflow after the other tool
// calls of the same step. A script_path call then reads the file after a
// write_file or edit_files call in that step has changed it.
type serialWorkflowTool struct{ fantasy.AgentTool }

func (serialWorkflowTool) SerialToolCalls() bool { return true }

// workflowScript returns args.Script, or the content of the workspace file
// at args.ScriptPath. The run compiles this text one time, so a later edit
// of the file changes only the next run.
func workflowScript(
	ctx context.Context,
	args runWorkflowArgs,
	getConn func(context.Context) (workspacesdk.AgentConn, error),
) (string, error) {
	path := strings.TrimSpace(args.ScriptPath)
	if path == "" {
		return args.Script, nil
	}
	if args.Script != "" {
		return "", xerrors.New("send script or script_path, not both")
	}
	conn, err := getConn(ctx)
	if err != nil {
		return "", xerrors.Errorf("read script_path: %w", err)
	}
	// One byte more than the limit tells an oversized file from a full one.
	rc, _, err := conn.ReadFile(ctx, path, 0, maxScriptBytes+1)
	if err != nil {
		return "", xerrors.Errorf("read script_path %q: %w", path, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return "", xerrors.Errorf("read script_path %q: %w", path, err)
	}
	if len(data) > maxScriptBytes {
		return "", xerrors.Errorf("the file %q is larger than %d bytes", path, maxScriptBytes)
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", xerrors.Errorf("the file %q is empty; write the script to it first", path)
	}
	return string(data), nil
}

func workflowJSON(res runWorkflowResult) fantasy.ToolResponse {
	out, err := json.Marshal(res)
	if err != nil {
		return fantasy.NewTextErrorResponse("marshal workflow result: " + err.Error())
	}
	return fantasy.NewTextResponse(string(out))
}

// workflowRunID derives the run ID from the tool call ID. Every replica
// and every retry of the call computes the same value.
func workflowRunID(callID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(callID, []byte("workflow-run"))
}

// workflowStepID gives the same child chat ID for the same spec in the
// same run. n counts earlier calls with the same spec, so a repeat is a
// new step.
func workflowStepID(runID uuid.UUID, specKey string, n int) uuid.UUID {
	return uuid.NewSHA1(runID, fmt.Appendf(nil, "%s#%d", specKey, n))
}

// workflowRegistry holds the runs of this replica.
type workflowRegistry struct {
	mu   sync.Mutex
	runs map[uuid.UUID]*workflowRun
}

func (g *workflowRegistry) get(id uuid.UUID) *workflowRun {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.runs[id]
}

// add stores run unless a run with its ID exists. It returns the stored run.
func (g *workflowRegistry) add(run *workflowRun) (*workflowRun, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if existing, ok := g.runs[run.id]; ok {
		return existing, false
	}
	if g.runs == nil {
		g.runs = map[uuid.UUID]*workflowRun{}
	}
	g.runs[run.id] = run
	return run, true
}

func (g *workflowRegistry) remove(id uuid.UUID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.runs, id)
}

// stepSpec is the input of agent() and spawn(). The title is not part of
// the step key, so a new title reuses a finished step.
type stepSpec struct {
	Prompt string          `json:"prompt"`
	Type   string          `json:"type"`
	Schema json.RawMessage `json:"schema"`
	Title  string          `json:"-"`
}

func (s stepSpec) key() (string, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return "", xerrors.Errorf("marshal step spec: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

type workflowStep struct {
	chatID uuid.UUID
	title  string
	reused bool
	// The fields below are guarded by workflowRun.mu.
	status string
	value  json.RawMessage
	err    string
}

type workflowRun struct {
	p             *Server
	id            uuid.UUID
	parent        database.Chat // the parent at start
	modelConfigID uuid.UUID     // the model of the turn that started the run
	program       *workflowProgram
	argsJSON      string

	// seen is owned by the script goroutine.
	seen map[string]int

	mu           sync.Mutex
	steps        []*workflowStep
	logs         []string
	result       *runWorkflowResult
	done         chan struct{}
	awaiters     int
	delivered    bool  // an await_workflow call returned the end
	notified     bool  // the parent got the end message
	queuedNotice int64 // the queued end message, 0 if none
}

func (p *Server) startWorkflowRun(
	ctx context.Context,
	chat database.Chat,
	modelConfigID uuid.UUID,
	args runWorkflowArgs,
	callID uuid.UUID,
) (*workflowRun, error) {
	if strings.TrimSpace(args.Script) == "" {
		return nil, xerrors.New("script is required")
	}
	if len(args.Script) > maxScriptBytes {
		return nil, xerrors.Errorf("the script is %d bytes; at most %d bytes are allowed", len(args.Script), maxScriptBytes)
	}
	runID := workflowRunID(callID)
	if strings.TrimSpace(args.ResumeRunID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(args.ResumeRunID))
		if err != nil {
			return nil, xerrors.New("resume_run_id must be a valid UUID")
		}
		runID = id
	}
	if run := p.workflows.get(runID); run != nil {
		if run.parent.ID != chat.ID {
			return nil, xerrors.New("the run belongs to another chat")
		}
		return run, nil
	}
	parent, err := p.loadSubagentSpawnParentChat(ctx, func() database.Chat { return chat })
	if err != nil {
		return nil, err
	}
	argsJSON := "{}"
	if args.Args != nil {
		raw, err := json.Marshal(args.Args)
		if err != nil {
			return nil, xerrors.Errorf("marshal args: %w", err)
		}
		argsJSON = string(raw)
	}
	program, err := compileWorkflow(args.Script)
	if err != nil {
		return nil, err
	}
	run, added := p.workflows.add(&workflowRun{
		p:             p,
		id:            runID,
		parent:        parent,
		modelConfigID: modelConfigID,
		program:       program,
		argsJSON:      argsJSON,
		seen:          map[string]int{},
		done:          make(chan struct{}),
	})
	if !added {
		return run, nil
	}
	// The run outlives the tool call. A fresh context drops the tool call
	// ID: the workspace agent answers a repeated ID with a saved response.
	// The run does the same chatd work as spawn_agent and wait_agent, which
	// also run as the chatd subject.
	runCtx, stop := p.inflightChatContext(dbauthz.AsChatd(context.Background()), parent) //nolint:gocritic // see above
	err = p.goInflight(func() {
		defer stop()
		run.finish(runCtx, run.execute(runCtx))
	})
	if err != nil {
		stop()
		p.workflows.remove(runID)
		return nil, xerrors.Errorf("start workflow run: %w", err)
	}
	return run, nil
}

// execute runs the script and builds the final result.
func (r *workflowRun) execute(ctx context.Context) runWorkflowResult {
	value, err := r.program.run(ctx, r)
	r.mu.Lock()
	defer r.mu.Unlock()
	res := r.snapshotLocked()
	if err != nil {
		res.Status = workflowStatusFailed
		res.Error = err.Error()
		return res
	}
	res.Status = workflowStatusCompleted
	res.Result = value
	return res
}

// finish stores the result. If no await_workflow call waits, it tells the
// parent that the run ended.
func (r *workflowRun) finish(ctx context.Context, res runWorkflowResult) {
	r.mu.Lock()
	r.result = &res
	close(r.done)
	waiting := r.awaiters > 0
	r.notified = !waiting
	r.mu.Unlock()
	// A stopped server must not send a message. The model resumes the run.
	if waiting || ctx.Err() != nil {
		return
	}
	r.notify(ctx)
}

func (r *workflowRun) notify(ctx context.Context) {
	r.mu.Lock()
	status := r.result.Status
	r.mu.Unlock()
	text := fmt.Sprintf("Workflow run %s ended with status %q. Call %s with run_id %q to read the result.",
		r.id, status, awaitWorkflowToolName, r.id.String())
	sent, err := r.p.SendMessage(ctx, SendMessageOptions{
		ChatID:       r.parent.ID,
		CreatedBy:    r.parent.OwnerID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)},
		BusyBehavior: SendMessageBusyBehaviorQueue, // an idle parent starts a new turn
	})
	if err != nil {
		r.p.logger.Warn(ctx, "send workflow end message",
			slog.F("chat_id", r.parent.ID), slog.F("run_id", r.id), slog.Error(err))
		return
	}
	if !sent.Queued || sent.QueuedMessage == nil {
		return
	}
	r.mu.Lock()
	deleteNow := r.delivered
	if !deleteNow {
		r.queuedNotice = sent.QueuedMessage.ID
	}
	r.mu.Unlock()
	if deleteNow {
		r.deleteNotice(ctx, sent.QueuedMessage.ID)
	}
}

func (r *workflowRun) deleteNotice(ctx context.Context, id int64) {
	if err := r.p.DeleteQueued(ctx, r.parent.ID, id); err != nil {
		r.p.logger.Debug(ctx, "delete workflow end message",
			slog.F("chat_id", r.parent.ID), slog.F("run_id", r.id), slog.Error(err))
	}
}

// await waits for the end of the run, at most for wait.
func (r *workflowRun) await(ctx context.Context, wait time.Duration) runWorkflowResult {
	r.mu.Lock()
	r.awaiters++
	r.mu.Unlock()

	timer := r.p.clock.NewTimer(wait, "chatd", "workflow_await")
	defer timer.Stop()
	select {
	case <-r.done:
	case <-timer.C:
	case <-ctx.Done():
	}

	r.mu.Lock()
	r.awaiters--
	ended := r.result != nil
	var res runWorkflowResult
	if ended {
		res = *r.result
	} else {
		res = r.snapshotLocked()
	}
	var noticeID int64
	sendNotice := false
	switch {
	case ended && ctx.Err() == nil:
		r.delivered = true
		noticeID, r.queuedNotice = r.queuedNotice, 0
	case ended && !r.delivered && !r.notified && r.awaiters == 0:
		// finish saw this call and sent no message, but the call stopped.
		r.notified = true
		sendNotice = true
	}
	r.mu.Unlock()

	if noticeID != 0 {
		r.deleteNotice(ctx, noticeID)
	}
	if sendNotice {
		// The message outlives the tool call, as in startWorkflowRun.
		noticeCtx, stop := r.p.inflightChatContext(dbauthz.AsChatd(context.Background()), r.parent) //nolint:gocritic // see startWorkflowRun
		if err := r.p.goInflight(func() {
			defer stop()
			r.notify(noticeCtx)
		}); err != nil {
			stop()
		}
	}
	return res
}

func (r *workflowRun) snapshot() runWorkflowResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.result != nil {
		return *r.result
	}
	return r.snapshotLocked()
}

func (r *workflowRun) snapshotLocked() runWorkflowResult {
	res := runWorkflowResult{
		RunID:  r.id.String(),
		Status: workflowStatusRunning,
		Steps:  make([]stepSummary, 0, len(r.steps)),
		Logs:   append([]string(nil), r.logs...),
	}
	for _, s := range r.steps {
		res.Steps = append(res.Steps, stepSummary{
			ChatID: s.chatID.String(),
			Title:  s.title,
			Status: s.status,
			Reused: s.reused,
			Error:  s.err,
		})
	}
	return res
}

func (r *workflowRun) log(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.appendLogLocked(msg)
}

func (r *workflowRun) appendLogLocked(msg string) {
	r.logs = append(r.logs, msg)
	if len(r.logs) > maxWorkflowLogs {
		r.logs = r.logs[len(r.logs)-maxWorkflowLogs:]
	}
}

// startStep reuses the child chat of a step that an earlier runner started,
// or it creates the child chat.
func (r *workflowRun) startStep(ctx context.Context, spec stepSpec) (*workflowStep, error) {
	switch spec.Type {
	case "":
		spec.Type = subagentTypeGeneral
	case subagentTypeGeneral, subagentTypeExplore:
	default:
		return nil, xerrors.Errorf("type must be %q or %q", subagentTypeGeneral, subagentTypeExplore)
	}
	if len(spec.Schema) == 0 {
		spec.Schema = json.RawMessage(defaultStepSchema)
	}
	if _, err := checkStepSchema(spec.Schema); err != nil {
		return nil, err
	}
	r.mu.Lock()
	count := len(r.steps)
	r.mu.Unlock()
	if count >= maxWorkflowSteps {
		return nil, xerrors.Errorf("a run can create %d steps at most", maxWorkflowSteps)
	}

	key, err := spec.key()
	if err != nil {
		return nil, err
	}
	id := workflowStepID(r.id, key, r.seen[key])
	r.seen[key]++

	reused := true
	child, err := r.p.db.GetChatByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		reused = false
		child, err = r.createStepChat(ctx, id, spec)
		if database.IsUniqueViolation(err, database.UniqueChatsPkey) {
			reused = true
			child, err = r.p.db.GetChatByID(ctx, id) // another runner created it
		}
	}
	if err != nil {
		return nil, xerrors.Errorf("start step %q: %w", spec.Title, err)
	}
	if child.ParentChatID.UUID != r.parent.ID {
		return nil, xerrors.Errorf("step chat %s belongs to another chat", id)
	}
	step := &workflowStep{chatID: child.ID, title: child.Title, reused: reused, status: stepStatusRunning}
	r.mu.Lock()
	r.steps = append(r.steps, step)
	r.mu.Unlock()
	return step, nil
}

func (r *workflowRun) createStepChat(ctx context.Context, id uuid.UUID, spec stepSpec) (database.Chat, error) {
	parent, err := r.p.loadSubagentSpawnParentChat(ctx, func() database.Chat { return r.parent })
	if err != nil {
		return database.Chat{}, err
	}
	def, err := resolveSubagentDefinition(ctx, r.p, parent, spec.Type)
	if err != nil {
		return database.Chat{}, err
	}
	options, err := def.buildOptions(ctx, r.p, parent, r.parent, r.modelConfigID, nil, spec.Prompt)
	if err != nil {
		return database.Chat{}, err
	}
	tools, err := workflowStepTools(spec.Schema)
	if err != nil {
		return database.Chat{}, err
	}
	options.chatID = id
	options.labels = database.StringMap{workflowRunLabel: r.id.String()}
	options.dynamicTools.RawMessage, options.dynamicTools.Valid = tools, true
	options.systemPrompt = strings.TrimSpace(options.systemPrompt + "\n\n" + workflowStepPrompt)
	// A step must reach workflow_report, so it never runs in plan mode.
	options.planModeOverride = &database.NullChatPlanMode{}
	return r.p.createChildSubagentChatWithOptions(ctx, parent, spec.Prompt, spec.Title, options)
}

// waitStep blocks until the step has a value or fails. A child that stops
// without a value gets one reminder.
func (r *workflowRun) waitStep(ctx context.Context, step *workflowStep) error {
	deadline := r.p.clock.Now().Add(workflowStepTimeout)
	for {
		r.mu.Lock()
		status := step.status
		r.mu.Unlock()
		if status == stepStatusReported || status == stepStatusFailed {
			return nil
		}
		if !r.p.clock.Now().Before(deadline) {
			r.failStep(step, "the step ran longer than "+workflowStepTimeout.String())
			return nil
		}
		_, _, err := r.p.awaitSubagentCompletion(ctx, r.parent.ID, step.chatID, defaultSubagentWaitTimeout)
		if errors.Is(err, ErrSubagentWaitTimeout) {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if statusErr, ok := errors.AsType[*subagentStatusError](err); ok {
			r.failStep(step, "the child chat failed: "+statusErr.reason)
			return nil
		}
		if err != nil {
			r.failStep(step, err.Error())
			return nil
		}
		value, reminded, err := workflowStepReport(ctx, r.p.db, step.chatID)
		if err != nil {
			return err
		}
		switch {
		case value != nil:
			r.mu.Lock()
			step.status, step.value = stepStatusReported, value
			r.mu.Unlock()
		case !reminded:
			if _, err := r.p.sendSubagentMessage(ctx, r.parent.ID, step.chatID,
				workflowReminder, SendMessageBusyBehaviorQueue); err != nil {
				r.failStep(step, "send reminder: "+err.Error())
				return nil
			}
			r.mu.Lock()
			step.status = stepStatusReminded
			r.mu.Unlock()
		default:
			r.failStep(step, "the step stopped without a "+workflowReportToolName+" call")
		}
	}
}

func (r *workflowRun) failStep(step *workflowStep, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	step.status, step.err = stepStatusFailed, reason
	r.appendLogLocked(fmt.Sprintf("step %q failed: %s", step.title, reason))
}

// stepValue returns the value of a finished step. ok is false for a
// failed step.
func (r *workflowRun) stepValue(step *workflowStep) (value json.RawMessage, reason string, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return step.value, step.err, step.status == stepStatusReported
}
