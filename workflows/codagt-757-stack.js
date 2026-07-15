export const meta = {
  description:
    "CODAGT-757: implement 3 stacked PRs (chatd execution records, agent idempotency token, attempt watchdog) and drive each through the Codex review loop and CI until mergeable.",
};

const MAX_CYCLES = 8;
const MAX_FIXUPS = 2;

const ENV_SETUP = [
  "Environment setup (do this first, in order):",
  "- You are in a fresh forked child workspace of github.com/coder/coder (git worktree, SHALLOW clone).",
  "- Run 'mise trust' in the repo root immediately; otherwise the pre-commit hook fails at lint/actions/actionlint with a not-trusted config error.",
  "- Run 'git fetch origin main --deepen=200' before your first commit or rebase. If 'git merge-base HEAD origin/main' fails, deepen further (400, then 800). scripts/check_emdash.sh and rebases need a resolvable merge-base.",
  "- gh CLI is authenticated and origin is coder/coder. NEVER push to origin/main or origin/master. Push only the branches named in this prompt. Force-push only with --force-with-lease and only to those branches.",
  "- Git hooks are mandatory. NEVER use --no-verify. The first hook run is slow while caches warm; wait it out.",
  "- No em dashes or en dashes anywhere: code, comments, commit messages, PR text. make lint/emdash enforces this. Use commas, periods, colons, or parentheses.",
  "- Commit message format: type(scope): message. The scope must be a real filesystem path containing every changed file; omit the scope for cross-cutting changes.",
  "- If 'make fmt' rewrites files you did not touch, restore those files to their base bytes. PRs must not contain whitespace-only or line-ending churn.",
].join("\n");

const REPO_RULES = [
  "Repository rules:",
  "- AGENTS.md and .claude/docs/WORKFLOWS.md govern the workflow: make gen after database or generated-API changes (run make gen twice when generated TypeScript types change), make fmt, make lint, targeted tests. make pre-commit runs via the commit hook.",
  "- Read .claude/docs/DATABASE.md before migration work, .claude/docs/TESTING.md before test work, .claude/docs/GO.md for Go conventions.",
  "- Comments must describe behavior, not the change process. No comments like 'added per plan'.",
  "- Before finalizing, self-review your diff and remove slop: unnecessary comments, defensive checks for impossible states, dead code, inconsistent style.",
].join("\n");

const STACK_NOTE = [
  "Stack layout (3 PRs, this workflow owns all of them):",
  "- PR 1: branch mike/codagt-757-execution-records, base main. Durable per-tool-call execution records in Postgres, re-attach on retry, interrupt kill, execute timeout clamp.",
  "- PR 2: branch mike/codagt-757-agent-idempotency-token, base mike/codagt-757-execution-records. Agent-side idempotency token with response echo, reap-age fix.",
  "- PR 3: branch mike/codagt-757-attempt-watchdog, base mike/codagt-757-agent-idempotency-token. Resettable attempt idle timer plus 24h absolute cap.",
  "Context: Linear issue CODAGT-757 (chatd task retry respawns execute tool processes, leaving orphaned duplicates).",
].join("\n");

const LOCKED_DECISIONS = [
  "Locked design decisions (do not relitigate; use these to resolve ambiguity and to answer review feedback):",
  "- Idempotency key is the tool call ID, already durable in chat_messages before execution. Records are keyed (chat_id, tool_call_id). Never dedup by command string.",
  "- Record cleanup: best-effort delete AFTER commitGenerationStep succeeds, never inside the commit transaction. Stale rows are harmless because unresolvedToolCallsFromHistory never re-returns resolved calls. dbpurge TTL is the janitor.",
  "- NULL-handle rows (a prior attempt died between reserving the row and learning the process ID): do NOT blindly restart and do NOT command-match. If this attempt created the row, start. If the row pre-exists with NULL process_id, allow a short grace (about 60s, via retryable error), then commit an is_error tool result stating the process may have started but its handle was lost, state unknown, re-run only if safe.",
  "- Re-attach semantics: snapshot first. If the process exited, commit the real result even past the deadline. If still running and started_at plus the clamped timeout is in the future, block-wait the remainder. If the deadline passed, return the existing graceful timed-out result with background_process_id.",
  "- 404-only synthesis: only a definite HTTP 404 from ProcessOutput (agent reached, process unknown) produces the unknown-state error result. Transport errors, dial failures, cancellations, and 5xx stay retryable.",
  "- Interrupt kill: foreground executes only, best-effort, after the interrupt commit, short dial timeout (about 5s). Never let a slow agent dial delay the interrupt.",
  "- Watchdog (PR 3): unexported typed context key plus a no-op-safe kick helper, set in taskAttemptContext, kicked from chattool only after successful agent round-trips. Streaming code never kicks, so the 10m stream-silence guard and the 15m idle window for non-tool work are unchanged. Absolute cap 24h.",
  "- Echo is mandatory for trust (PR 2): a missing client_token echo in the StartProcess response means old agent, and coderd assumes no agent-side dedup happened.",
].join("\n");

const PLAN_PR1 = [
  "Implementation plan for PR 1 (all file and line references were verified against main shortly before this run; re-verify locations as you work):",
  "",
  "1. Migration: pick the actual next free number in coderd/database/migrations (000540 was next at planning time). Files NNNNNN_chat_tool_call_executions.{up,down}.sql. Up:",
  "",
  "    CREATE TABLE chat_tool_call_executions (",
  "        chat_id            UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,",
  "        tool_call_id       TEXT        NOT NULL,",
  "        workspace_agent_id UUID        REFERENCES workspace_agents(id) ON DELETE SET NULL,",
  "        process_id         TEXT,",
  "        command            TEXT        NOT NULL,",
  "        background         BOOLEAN     NOT NULL DEFAULT false,",
  "        timeout_secs       BIGINT      NOT NULL,",
  "        created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),",
  "        started_at         TIMESTAMPTZ,",
  "        PRIMARY KEY (chat_id, tool_call_id)",
  "    );",
  "    CREATE INDEX idx_chat_tool_call_executions_created_at ON chat_tool_call_executions (created_at);",
  "",
  "   Column notes as SQL comments or nearby doc: process_id NULL means reserved but not yet started; command is for mismatch diagnostics only, never dedup; timeout_secs is the clamped tool timeout at reserve time; started_at is set together with process_id. Down migration: DROP TABLE IF EXISTS chat_tool_call_executions; State is encoded by process_id nullability; do not add an enum type.",
  "",
  "2. Queries in coderd/database/queries/chattoolcallexecutions.sql:",
  "   - InsertChatToolCallExecution :one  (plain insert; the caller detects unique violation via database.IsUniqueViolation to learn created vs pre-existing; that is the compare-and-set that keeps two lease-handoff owners from both acting as creator)",
  "   - GetChatToolCallExecution :one  (chat_id + tool_call_id)",
  "   - GetChatToolCallExecutions :many  (chat_id + tool_call_ids array, for the interrupt path)",
  "   - UpdateChatToolCallExecutionProcess :one  (sets process_id, workspace_agent_id, started_at)",
  "   - DeleteChatToolCallExecutions :exec  (chat_id + tool_call_ids array)",
  "   - DeleteOldChatToolCallExecutions :execrows  (created_at cutoff, for dbpurge)",
  "   Run make gen (twice when generated TypeScript types change) and hand-fix dbauthz stubs as needed.",
  "",
  "3. dbauthz: follow the parent-chat pattern (coderd/database/dbauthz/dbauthz.go near lines 5879 to 5906): fetch the chat via GetChatByID and authorize policy.ActionUpdate for insert/update/delete, policy.ActionRead for gets. DeleteOldChatToolCallExecutions authorizes rbac.ResourceSystem with policy.ActionDelete (mirror DeleteOldChats). Add entries to the TestChats matrix in coderd/database/dbauthz/dbauthz_test.go (mocked pattern near lines 529 to 598). Do NOT touch enterprise/audit/table.go; this is an operational child table and the audited Chat model is unchanged.",
  "",
  "4. Recorder interface in coderd/x/chatd/chattool/execute.go. ExecuteOptions currently has GetWorkspaceConn and DefaultTimeout (lines 65 to 69). Add Recorder ExecutionRecorder; nil Recorder preserves legacy behavior so existing unit tests stay valid. Shape:",
  "",
  "    type ExecutionRecord struct {",
  "        ProcessID  string",
  "        Command    string",
  "        Background bool",
  "        Timeout    time.Duration",
  "        CreatedAt  time.Time",
  "        StartedAt  time.Time",
  "    }",
  "    type ExecutionRecorder interface {",
  "        Reserve(ctx context.Context, toolCallID string, command string, background bool, timeout time.Duration) (rec ExecutionRecord, created bool, err error)",
  "        RecordStart(ctx context.Context, toolCallID string, processID string) error",
  "    }",
  "",
  "   chatd implements it in a new coderd/x/chatd/execution_recorder.go closing over server.db, the chat ID, and the workspace agent ID of the active connection (see turnWorkspaceContext in chatd.go, agent resolution near line 740). Wire it at tool construction in coderd/x/chatd/generation_preparer.go lines 363 to 383, where server.db and the chat are in scope.",
  "",
  "5. Execute tool rework in coderd/x/chatd/chattool/execute.go:",
  "   - The Execute run closure (lines 92 to 105) currently discards the third fantasy.ToolCall parameter; use call.ID as the tool call ID.",
  "   - Timeout clamp first: reject timeout <= 0 with a text error response; clamp to a new named const maxExecuteTimeout = 4 hours. Apply the same clamp to the process_output tool wait_timeout (lines 422 to 443).",
  "   - New flow in executeForeground (lines 179 to 230) and the background path:",
  "     a. Reserve(...).",
  "     b. created == false and rec.ProcessID empty: NULL-handle row from a dead attempt. Poll the record briefly (about 60s grace); if still empty return a retryable error on first encounter; past grace, return the is_error unknown-state result per the locked decisions.",
  "     c. rec.ProcessID set: re-attach. Take a non-blocking snapshot via conn.ProcessOutput(ctx, id, nil). Process exited: build the normal completed result even if the deadline passed. Running and rec.StartedAt plus rec.Timeout still in the future: reuse the existing waitForProcess polling (lines 243 to 329) for the remainder. Deadline passed: return the existing graceful timed-out result with background_process_id. Definite HTTP 404 (codersdk.Error with StatusCode 404): return the unknown-state is_error result naming the process ID. Any other error: normal retryable error.",
  "     d. Fresh start (created == true): StartProcess, then RecordStart immediately. If RecordStart fails, return a retryable error and do not kill the running process.",
  "   - Background executes participate identically; re-attach for a background record returns the same started-in-background result shape.",
  "   - Keep changes surgical; do not refactor unrelated code.",
  "",
  "6. Post-commit cleanup in coderd/x/chatd/generation.go executeLocalTools (lines 633 to 668): after commitGenerationStep returns nil, best-effort delete the execution records for the executed tool call IDs. Log failures; never fail the task on cleanup errors.",
  "",
  "7. Interrupt kill in coderd/x/chatd/tasks.go StartInterrupt (lines 243 to 306): after the interrupt commit succeeds, load the records for the unresolved local tool call IDs (already computed by committedPendingLocalToolCancellationMessages, lines 674 to 700). For each foreground record with a process_id and agent binding, dial via server.agentConnFn with a short timeout (about 5s) and SignalProcess(id, \"kill\"). Best-effort: log failures, then delete those records.",
  "",
  "8. dbpurge: add DeleteOldChatToolCallExecutions with a 7-day cutoff const to coderd/database/dbpurge/dbpurge.go following the existing DeleteOld pattern (near lines 364 to 381); extend dbpurge_test.go.",
  "",
  "9. Update coderd/x/chatd/ARCHITECTURE.md: execution records, reserve and re-attach lifecycle, unknown-state semantics, interrupt kill. Mandatory in this repo when chatd architecture changes.",
  "",
  "10. Tests:",
  "   - coderd/x/chatd/chattool/execute_test.go using the existing gomock agent conn helper (near lines 644 to 651) plus a small fake in-memory recorder: fresh start records the process; retry with a recorded process re-attaches without a second StartProcess; re-attach whose snapshot shows exited commits the real result; re-attach past deadline returns the timed-out result with background id; 404 returns the unknown-state error; NULL-handle grace then unknown; clamp rejects 0s and 25h; nil recorder preserves legacy behavior.",
  "   - coderd/x/chatd/chatd_test.go integration test mirroring TestPersistToolResultWithBinaryData (lines 3016 to 3072): a tool call whose first attempt is canceled mid ProcessOutput, asserting exactly one StartProcess across the retry and a real committed tool result. An interrupt test (near line 3403) asserting SignalProcess was called.",
  "   - dbauthz matrix entries and a dbpurge test.",
  "",
  "Validation for this PR: make fmt, make lint, make gen with a clean diff after, then targeted tests: go test ./coderd/x/chatd/... -run with the relevant test names, go test ./coderd/database/dbauthz -run TestChats, and the dbpurge test. Postgres-backed tests follow .claude/docs/TESTING.md; if an environment limit prevents one locally, state that in the PR test plan and rely on CI.",
].join("\n");

const PLAN_PR2 = [
  "Implementation plan for PR 2 (verify locations as you work; line references were checked against main at planning time):",
  "",
  "1. SDK types in codersdk/workspacesdk/agentconn.go (near lines 841 to 852):",
  "   - StartProcessRequest gains ClientToken string, json tag client_token,omitempty.",
  "   - StartProcessResponse gains ClientToken string (echo of the accepted token) and Attached bool (true when an existing process was returned instead of starting a new one). Keep the existing Started field semantics.",
  "",
  "2. Agent side in agent/agentproc:",
  "   - manager (process.go near lines 74 to 85) gains a token index: a map keyed by chatID joined with the token (use a separator that cannot appear in either), value process ID.",
  "   - start() (process.go near lines 108 to 186): when a token is present and the index hits, fetch the process; if command, workdir, or background differ from the request, return a conflict error that the API layer maps to HTTP 409 (the EC2 IdempotentParameterMismatch analog); if they match, return the existing process with attached true. On miss, create and index.",
  "   - handleStartProcess (api.go near lines 65 to 119): pass the token through; respond with the echo and Attached; skip the path-store notification when attaching to an existing process.",
  "   - Reaping (process.go near lines 251 to 269): raise exitedProcessReapAge from 5 minutes to 60 minutes; remove token-index entries when reaping; also trigger the reap sweep from start() so the index cannot grow unbounded between list calls.",
  "",
  "3. coderd wiring in coderd/x/chatd/chattool/execute.go: set ClientToken to the tool call ID on every StartProcess. On response: if the echo is missing, log (once per connection is fine) that the agent lacks idempotent start support; behavior stays as PR 1, the Postgres record remains the source of truth. When the echo is present and Attached is true, log an execute_agent_deduped line.",
  "",
  "4. Tests: agent/agentproc/api_test.go using the existing httptest pattern (near lines 119 to 145): same-token second start returns the same process ID with attached true; same token with a different command returns 409; the token index entry is freed after reap; requests without a token behave exactly as before. chattool/execute_test.go: mock echoes the token or omits it; assert only logging differs.",
  "",
  "Validation: make fmt, make lint, go test ./agent/agentproc/... plus targeted chattool tests. No DB changes in this PR. If generated API or SDK docs require make gen, run it and commit the output.",
].join("\n");

const PLAN_PR3 = [
  "Implementation plan for PR 3 (verify locations as you work; line references were checked against main at planning time):",
  "",
  "1. coderd/x/chatd/tasks.go:",
  "   - Keep defaultTaskTimeout = 15m but redefine it as the attempt IDLE window; add maxTaskAttemptDuration = 24h as a non-resettable absolute cap.",
  "   - taskAttemptContext (near lines 156 to 164): keep the AfterFunc idle timer but make it resettable behind a small mutex-guarded helper (mirror the streamSilenceGuard pattern in coderd/x/chatd/chatloop/chatloop.go near lines 620 to 660); add a second non-resettable 24h AfterFunc; both cancel the attempt with errTaskTimeout.",
  "   - Update the comment near tasks.go lines 27 to 29 to describe idle-window semantics: the idle window must stay above the 10m stream-silence guard so silent provider streams keep failing through chat-specific retry first.",
  "",
  "2. Keepalive plumbing: define WithAttemptKeepalive(ctx, func()) and KickAttemptKeepalive(ctx) with an unexported typed context key in a new coderd/x/chatd/chattool/keepalive.go. chatd imports chattool and never the reverse, so there is no import cycle. KickAttemptKeepalive is a safe no-op when the value is absent or the attempt already ended.",
  "",
  "3. Kick sites, chattool only, after each SUCCESSFUL agent round-trip: StartProcess return, every ProcessOutput return inside waitForProcess (including rounds where the process is still running), process_output tool round-trips, and the background start path. Never kick from streaming code.",
  "",
  "4. runTaskWithRetry installs the keepalive func into the attempt context via chattool.WithAttemptKeepalive when creating the attempt context.",
  "",
  "5. Tests in coderd/x/chatd/tasks_test.go using quartz.NewMock (existing pattern near lines 34 to 48): an attempt with periodic kicks survives past 15m; it still dies at the 24h cap regardless of kicks; a no-kick attempt dies at 15m exactly as today; kicking after cancel is a safe no-op. Add a chattool test asserting KickAttemptKeepalive fires on successful poll rounds (inject the kick func via context in execute_test.go).",
  "",
  "6. Amend coderd/x/chatd/ARCHITECTURE.md for the idle-window watchdog and the absolute cap.",
  "",
  "Validation: make fmt, make lint, targeted go test ./coderd/x/chatd/... runs.",
].join("\n");

const CODEX_RULES = [
  "Codex review rules for coder/coder (verified behavior, follow exactly):",
  "- Trigger: post an issue comment whose body is exactly: @codex review",
  "- Bot identity: the author login CONTAINS chatgpt-codex-connector (REST shows the [bot] suffix; never exact-match the name without it).",
  "- Codex does NOT create check runs on this repo and does not react to the trigger comment. Its response is either a PR review (state COMMENTED, usually with inline threads) when it has feedback, or an issue comment containing wording like: Didn't find any major issues. That comment is the OK signal. Typical latency is 6 to 10 minutes; poll for up to 25 minutes before considering a nudge.",
  "- State machine, always evaluated against the CURRENT head SHA: needs-request (no trigger newer than the last head change) means post the trigger. awaiting-response means poll roughly every 60s in bounded chunks; only codex artifacts newer than the latest trigger on the current head count. Response with feedback means fix. OK signal means proceed to CI verification.",
  "- Retrigger cap: at most 2 trigger comments per head SHA. If the cap is reached and codex stays silent, keep polling and report progress honestly; never exit claiming success.",
  "- Feedback handling: fix each actionable item minimally and in scope. For false positives, reply on the thread with concrete technical reasoning. Resolve every addressed thread via the GraphQL resolveReviewThread mutation (find thread ids via the reviewThreads query, filtering codex-authored unresolved threads). Then commit (hooks must pass), push, and post the trigger again for the new head.",
  "- Completion for codex requires ALL of: a codex response newer than the latest trigger on the current head, the OK signal for that head, and zero unresolved codex-authored review threads.",
  "- gh pitfalls: gh --jq does not support jq --arg (interpolate values or pipe to real jq). Never combine 2>/dev/null with || fallbacks that hide persistent query errors. Keep command output small (the tool truncates past 300 lines).",
].join("\n");

const CI_RULES = [
  "CI verification rules:",
  "- Determine check status from the source of truth: gh api repos/coder/coder/commits/HEAD_SHA/check-runs --paginate, group by .name, take the run with the latest started_at per name, then count queued, in_progress, and failed. Do NOT trust gh pr view statusCheckRollup; it retains stale failed runs after re-runs.",
  "- Also require gh pr view --json mergeable to not be CONFLICTING.",
  "- Full CI takes 30 to 70 minutes. Poll in bounded chunks with sleep of about 120s between checks.",
  "- On a failed check: read gh run view RUN_ID --log-failed. If the failure is caused by this diff, fix it, commit, push, post the codex trigger again (the head changed), and continue. If it is clearly infra or a known flake unrelated to the diff, re-run with gh run rerun RUN_ID --failed (at most 2 re-runs per check name per head); if it still fails, report progress with the evidence instead of claiming done.",
  "- Known failure: migration number collision when main gains a migration with the same number. Fix by renumbering our migration files (and any references), re-running make gen if needed, commit, push, re-trigger codex.",
].join("\n");

const CYCLE_REPORTING = [
  "Budget and reporting:",
  "- Your cycle budget is about 90 minutes of wall clock. Do the setup, then advance the PR through as many steps as fit: restack if needed, ensure a codex request exists for the current head, wait for codex, fix feedback, verify CI, fix CI. When the remaining budget gets thin, do not start a new long wait; report status progress with the exact state: head SHA, latest trigger time, latest codex activity and what it was, unresolved thread count, CI counts, and what the next cycle should do first.",
  "- Report status done ONLY when every completion criterion is verified on the current head in a final refresh pass: codex OK signal newer than the latest trigger for this head, zero unresolved codex-authored threads, every latest-per-name check run concluded success, neutral, or skipped, mergeable not CONFLICTING, and the head SHA unchanged during that final pass.",
  "- Report status blocked ONLY for external blockers (gh auth failure, permissions, an instruction conflict) with a precise blocker description. A long wait is never a blocker.",
  "- Never merge, close, or retarget any PR. Never mark draft or ready except as instructed. Do not touch PRs other than the one assigned to you (reading parent PR state is fine).",
  "- If you push fixes, keep the PR description aligned with the full diff, and keep the disclosure blockquote at the end of the body: > Created by Mux on Mike's behalf.",
].join("\n");

const PR_SPECS = [
  {
    key: "pr1",
    branch: "mike/codagt-757-execution-records",
    base: "main",
    shortTitle: "chatd execution records + re-attach",
    titleHint:
      "fix(coderd): make chat execute tool starts idempotent under task retry",
    plan: PLAN_PR1,
    implementSoftMinutes: 300,
    restack: [
      "Restack policy for this PR (base is main): do NOT rebase onto main routinely. Only if gh pr view reports mergeable CONFLICTING: deepen the clone (git fetch origin main --deepen=400), rebase onto origin/main, resolve conflicts faithfully to the plan, force-push with --force-with-lease, and note that children of this PR will restack at their own turn.",
    ].join("\n"),
  },
  {
    key: "pr2",
    branch: "mike/codagt-757-agent-idempotency-token",
    base: "mike/codagt-757-execution-records",
    shortTitle: "agent idempotency token",
    titleHint:
      "feat: add idempotent start tokens to the workspace agent process API",
    plan: PLAN_PR2,
    implementSoftMinutes: 180,
    restack: [
      "Restack policy for this PR: at cycle start, git fetch origin, then check whether origin/mike/codagt-757-execution-records has commits not in this branch. If so, rebase this branch onto origin/mike/codagt-757-execution-records, resolve conflicts faithfully to the plan, and force-push with --force-with-lease. A restack changes the head, which resets codex state; that is expected, post the trigger again.",
    ].join("\n"),
  },
  {
    key: "pr3",
    branch: "mike/codagt-757-attempt-watchdog",
    base: "mike/codagt-757-agent-idempotency-token",
    shortTitle: "attempt watchdog",
    titleHint:
      "fix(coderd/x/chatd): keep task attempts alive while execute tools make progress",
    plan: PLAN_PR3,
    implementSoftMinutes: 150,
    restack: [
      "Restack policy for this PR: at cycle start, git fetch origin, then check whether origin/mike/codagt-757-agent-idempotency-token has commits not in this branch. If so, rebase this branch onto origin/mike/codagt-757-agent-idempotency-token, resolve conflicts faithfully to the plan, and force-push with --force-with-lease. A restack changes the head, which resets codex state; that is expected, post the trigger again.",
    ].join("\n"),
  },
];

function implementSchema() {
  return {
    type: "object",
    required: ["status", "prNumber", "prUrl", "branch", "headSha", "summary"],
    additionalProperties: false,
    properties: {
      status: { type: "string", enum: ["created", "existing", "partial", "blocked"] },
      prNumber: { type: "number" },
      prUrl: { type: "string" },
      branch: { type: "string" },
      headSha: { type: "string" },
      summary: { type: "string" },
      blocker: { type: ["string", "null"] },
    },
  };
}

function cycleSchema() {
  return {
    type: "object",
    required: [
      "status",
      "prNumber",
      "headSha",
      "codexOk",
      "ciGreen",
      "unresolvedCodexThreads",
      "summary",
    ],
    additionalProperties: false,
    properties: {
      status: { type: "string", enum: ["done", "progress", "blocked"] },
      prNumber: { type: "number" },
      headSha: { type: "string" },
      codexOk: { type: "boolean" },
      ciGreen: { type: "boolean" },
      unresolvedCodexThreads: { type: "number" },
      summary: { type: "string" },
      nextAction: { type: ["string", "null"] },
      blocker: { type: ["string", "null"] },
    },
  };
}

function verifySchema() {
  return {
    type: "object",
    required: ["allOk", "prs", "summary"],
    additionalProperties: false,
    properties: {
      allOk: { type: "boolean" },
      prs: {
        type: "array",
        items: {
          type: "object",
          required: ["key", "prNumber", "ok"],
          additionalProperties: false,
          properties: {
            key: { type: "string" },
            prNumber: { type: "number" },
            ok: { type: "boolean" },
            issue: { type: ["string", "null"] },
          },
        },
      },
      summary: { type: "string" },
    },
  };
}

function implementPrompt(pr) {
  return [
    "Task: implement one PR of a 3-PR stack for Linear issue CODAGT-757 in coder/coder, push the branch, and open the PR. Do NOT interact with Codex; a later phase owns review.",
    "",
    STACK_NOTE,
    "",
    "Your PR: " + pr.key + " on branch " + pr.branch + " with base " + pr.base + ".",
    "",
    ENV_SETUP,
    "",
    REPO_RULES,
    "",
    "Idempotency first: check 'git ls-remote --heads origin " + pr.branch + "' and 'gh pr list --head " + pr.branch + " --state open --json number,url'. If the branch and PR already exist from an earlier interrupted run, fetch the branch, review its state against the plan, and finish the remaining work on top instead of starting over. Report status existing in that case (after completing any missing work).",
    "",
    "Branching: git fetch origin " + pr.base + " (and main), then create the branch exactly named " + pr.branch + " from origin/" + pr.base + ". Never reuse the workspace's own default branch name.",
    "",
    LOCKED_DECISIONS,
    "",
    pr.plan,
    "",
    "PR creation once validation passes:",
    "- Push with git push -u origin " + pr.branch + ".",
    "- Create a non-draft PR with gh pr create --base " + pr.base + " (the requester explicitly wants ready, mergeable PRs).",
    "- Title: follow the repo convention type(scope): message with a valid path scope. Suggested: " + pr.titleHint + " (adjust if the final diff makes a different scope more accurate).",
    "- Body: follow .claude/docs/PR_STYLE_GUIDE.md. Describe the whole diff against the base. Do not hard-wrap prose. Include: the problem (one paragraph, referencing the duplicate-process incident class), what this PR changes, what it deliberately does not change, a test plan section listing exactly what you ran, a stack note (which part of the 3-PR stack this is and which PR it depends on), and the line: Part of CODAGT-757.",
    "- End the body with this exact blockquote line: > Created by Mux on Mike's behalf.",
    "",
    "Acceptance before reporting: branch pushed; PR open against the right base; make fmt, make lint, and make gen leave no dirty diff; the targeted tests listed in the plan pass locally (or the report and PR body state exactly which could not run locally and why); commits satisfy the hooks without --no-verify.",
    "",
    "Report via agent_report with: status (created when you made the PR now, existing when resuming found it complete, partial when out of time with work remaining, blocked only for external blockers), prNumber, prUrl, branch, headSha (git rev-parse HEAD after the final push), summary (what was implemented, validation results, anything notable for reviewers), and blocker when applicable. Use 0 and empty strings for unknown numeric or string fields.",
  ].join("\n");
}

function continuationPrompt(pr, prevSummary) {
  return [
    "Task: FINISH an interrupted implementation of " + pr.key + " (branch " + pr.branch + ", base " + pr.base + ") for CODAGT-757 in coder/coder. A previous agent ran out of time. Its final state report follows; verify it against the actual branch state, then complete the remaining work, validation, push, and PR creation.",
    "",
    "Previous state report:",
    prevSummary,
    "",
    "Everything below is the same brief the previous agent had.",
    "",
    implementPrompt(pr),
  ].join("\n");
}

function cyclePrompt(pr, iteration, prNumber, prevState) {
  return [
    "Task: advance PR #" + prNumber + " (" + pr.shortTitle + ", branch " + pr.branch + ", base " + pr.base + ") in coder/coder toward review-complete, then report honestly. This is cycle " + iteration + " of at most " + MAX_CYCLES + " for this PR; a durable workflow re-invokes cycles until done, so a truthful progress report is as valuable as completion.",
    "",
    STACK_NOTE,
    "",
    ENV_SETUP,
    "",
    "Setup: git fetch origin, then git checkout -B " + pr.branch + " origin/" + pr.branch + ". Confirm the PR number with gh pr list --head " + pr.branch + " --state open --json number and verify it is " + prNumber + ".",
    "",
    pr.restack,
    "",
    CODEX_RULES,
    "",
    CI_RULES,
    "",
    CYCLE_REPORTING,
    "",
    "Design reference for judging and fixing feedback (do not relitigate locked decisions; push back on review suggestions that contradict them, with reasoning, on the thread):",
    "",
    LOCKED_DECISIONS,
    "",
    pr.plan,
    "",
    prevState
      ? "Previous cycle state (verify against live state before acting on it):\n" + prevState
      : "This is the first review cycle for this PR. Codex has not been requested yet for its current head; expect to post the first trigger after validating the PR state.",
    "",
    "Report via agent_report with: status (done, progress, or blocked), prNumber, headSha, codexOk, ciGreen, unresolvedCodexThreads, summary, nextAction (what the next cycle should do first, null when done), blocker (null unless blocked).",
  ].join("\n");
}

function verifyPrompt(prs) {
  const lines = prs.map(function (p) {
    return "- " + p.key + ": PR #" + p.prNumber + " branch " + p.branch + " base " + p.base;
  });
  return [
    "Task: read-only final verification of a 3-PR stack in coder/coder. For each PR below, verify ALL completion criteria on its current head SHA and report per-PR ok plus an overall allOk.",
    "",
    lines.join("\n"),
    "",
    "Criteria per PR:",
    "- Open, non-draft, correct base branch.",
    "- gh pr view --json mergeable is not CONFLICTING.",
    "- Latest-per-name check runs on the head commit (gh api repos/coder/coder/commits/SHA/check-runs --paginate, group by name, take latest started_at per name) all concluded success, neutral, or skipped; none queued or in_progress.",
    "- Codex satisfied for the current head: an issue comment or review from an author whose login contains chatgpt-codex-connector, newer than the latest '@codex review' trigger comment, with the OK wording (like: Didn't find any major issues) for that head, and zero unresolved codex-authored review threads.",
    "- For pr2 and pr3: the branch contains the current tip of its base branch (no pending restack).",
    "",
    "Use only read operations (gh api, gh pr view, git ls-remote). Do not push, comment, or modify anything.",
    "",
    "Report via agent_report with: allOk, prs (key, prNumber, ok, issue describing any failed criterion), summary.",
  ].join("\n");
}

function truncate(text, max) {
  if (typeof text !== "string") return "";
  return text.length > max ? text.slice(0, max) + " [truncated]" : text;
}

function failReport(stage, details) {
  return {
    reportMarkdown: [
      "# CODAGT-757 stack workflow: stopped before completion",
      "",
      "Stage: " + stage,
      "",
      "Details:",
      "",
      "```json",
      JSON.stringify(details, null, 2),
      "```",
      "",
      "The workflow ended without meeting the full completion criteria. Inspect the details, fix the blocker, and resume or restart the remaining work.",
    ].join("\n"),
    structuredOutput: { status: "failed", stage: stage, details: details },
  };
}

export default function workflow({ phase, log, agent }) {
  const state = {};

  for (const pr of PR_SPECS) {
    phase("implement-" + pr.key, { branch: pr.branch, base: pr.base });
    let impl = agent(implementPrompt(pr), {
      id: "implement-" + pr.key,
      title: "Implement " + pr.shortTitle,
      schema: implementSchema(),
      timeout: {
        softMs: pr.implementSoftMinutes * 60000,
        graceMs: 10 * 60000,
        finalInstructions:
          "Time is up. Stop starting new work. Commit and push whatever is complete and internally consistent, then report honestly via agent_report. Use status partial if the PR is not fully created and validated, and describe precisely what remains.",
      },
    });
    if (impl.status === "partial") {
      log("Implementation ran out of time, launching continuation", {
        pr: pr.key,
        summary: truncate(impl.summary, 400),
      });
      impl = agent(continuationPrompt(pr, truncate(impl.summary, 3000)), {
        id: "implement-" + pr.key + "-cont",
        title: "Finish " + pr.shortTitle,
        schema: implementSchema(),
        timeout: {
          softMs: 180 * 60000,
          graceMs: 10 * 60000,
          finalInstructions:
            "Time is up. Report honestly via agent_report with status partial or blocked and exact remaining work.",
        },
      });
    }
    if (impl.status !== "created" && impl.status !== "existing") {
      return failReport("implement-" + pr.key, impl);
    }
    state[pr.key] = { impl: impl, cycles: [] };
    log("PR ready for review phase", {
      pr: pr.key,
      number: impl.prNumber,
      url: impl.prUrl,
      head: impl.headSha,
    });
  }

  for (const pr of PR_SPECS) {
    const prNumber = state[pr.key].impl.prNumber;
    phase("review-loop-" + pr.key, { prNumber: prNumber, branch: pr.branch });
    let finished = false;
    let prevState = "";
    for (let i = 1; i <= MAX_CYCLES && !finished; i++) {
      const cycle = agent(cyclePrompt(pr, i, prNumber, prevState), {
        id: "cycle-" + pr.key + "-" + i,
        title: "Review cycle " + i + ": " + pr.shortTitle,
        schema: cycleSchema(),
        timeout: {
          softMs: 100 * 60000,
          graceMs: 8 * 60000,
          finalInstructions:
            "Time is up. Do not start new waits or pushes. Report the exact current state via agent_report with status progress (or done only if every criterion is already verified).",
        },
      });
      state[pr.key].cycles.push(cycle);
      log("Cycle result", {
        pr: pr.key,
        cycle: i,
        status: cycle.status,
        head: cycle.headSha,
        codexOk: cycle.codexOk,
        ciGreen: cycle.ciGreen,
        unresolvedThreads: cycle.unresolvedCodexThreads,
      });
      if (cycle.status === "done") {
        finished = true;
      } else if (cycle.status === "blocked") {
        return failReport("review-loop-" + pr.key + "-cycle-" + i, cycle);
      } else {
        prevState = truncate(
          cycle.summary + (cycle.nextAction ? "\nNext action: " + cycle.nextAction : ""),
          3000
        );
      }
    }
    if (!finished) {
      return failReport("review-loop-" + pr.key + "-bounded-out", {
        prNumber: prNumber,
        lastCycle: state[pr.key].cycles[state[pr.key].cycles.length - 1],
      });
    }
  }

  phase("final-verify", {});
  const verifyInput = PR_SPECS.map(function (pr) {
    return {
      key: pr.key,
      branch: pr.branch,
      base: pr.base,
      prNumber: state[pr.key].impl.prNumber,
    };
  });
  let verify = agent(verifyPrompt(verifyInput), {
    id: "verify-1",
    title: "Final stack verification",
    schema: verifySchema(),
    agentId: "explore",
    timeout: {
      softMs: 30 * 60000,
      graceMs: 5 * 60000,
      finalInstructions: "Report current findings via agent_report now.",
    },
  });

  if (!verify.allOk) {
    for (const bad of verify.prs.filter(function (p) { return !p.ok; })) {
      const pr = PR_SPECS.find(function (p) { return p.key === bad.key; });
      if (!pr) continue;
      let fixed = false;
      for (let i = 1; i <= MAX_FIXUPS && !fixed; i++) {
        const cycle = agent(
          cyclePrompt(pr, MAX_CYCLES + i, state[pr.key].impl.prNumber,
            "Final verification found this PR not ready: " + (bad.issue || "unspecified") + ". Fix that and re-verify all completion criteria."),
          {
            id: "cycle-" + pr.key + "-fix-" + i,
            title: "Fixup cycle " + i + ": " + pr.shortTitle,
            schema: cycleSchema(),
            timeout: {
              softMs: 100 * 60000,
              graceMs: 8 * 60000,
              finalInstructions:
                "Time is up. Report the exact current state via agent_report with status progress (or done only if fully verified).",
            },
          }
        );
        state[pr.key].cycles.push(cycle);
        if (cycle.status === "done") fixed = true;
        if (cycle.status === "blocked") return failReport("fixup-" + pr.key, cycle);
      }
      if (!fixed) {
        return failReport("fixup-" + pr.key + "-bounded-out", { issue: bad.issue });
      }
    }
    verify = agent(verifyPrompt(verifyInput), {
      id: "verify-2",
      title: "Final stack verification (second pass)",
      schema: verifySchema(),
      agentId: "explore",
      timeout: {
        softMs: 30 * 60000,
        graceMs: 5 * 60000,
        finalInstructions: "Report current findings via agent_report now.",
      },
    });
    if (!verify.allOk) {
      return failReport("final-verify-failed", verify);
    }
  }

  phase("complete", {});
  const rows = PR_SPECS.map(function (pr) {
    const impl = state[pr.key].impl;
    const lastCycle = state[pr.key].cycles[state[pr.key].cycles.length - 1];
    return (
      "| " + pr.key + " | #" + impl.prNumber + " | " + pr.branch + " | " + pr.base +
      " | " + (lastCycle ? lastCycle.headSha : impl.headSha) + " | " + impl.prUrl + " |"
    );
  });
  const report = [
    "# CODAGT-757 stack: all PRs review-complete",
    "",
    "| PR | Number | Branch | Base | Head | URL |",
    "| --- | --- | --- | --- | --- | --- |",
    rows.join("\n"),
    "",
    "Every PR satisfies: codex OK signal on the current head, zero unresolved codex threads, all latest check runs green, mergeable, correct stacked base.",
    "",
    "Verification summary: " + verify.summary,
  ].join("\n");

  return {
    reportMarkdown: report,
    structuredOutput: {
      status: "complete",
      prs: PR_SPECS.map(function (pr) {
        return {
          key: pr.key,
          prNumber: state[pr.key].impl.prNumber,
          url: state[pr.key].impl.prUrl,
          branch: pr.branch,
          base: pr.base,
        };
      }),
    },
  };
}
