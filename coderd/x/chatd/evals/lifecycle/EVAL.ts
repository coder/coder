import { defineExperiment } from "@coder/aegis";
import { configureSuite, lifecycleTools, workspaceTools, type Case, type Check } from "../shared.ts";

// Workspace lifecycle: when chatd creates, starts, stops, or leaves alone a
// workspace. Expectations trace to the workspace awareness text and the
// <tool-use> and <workspace-template-selection> sections of
// coderd/x/chatd/prompt.go; README.md maps each case to its line.

const detached = { AEGIS_CODERAGENTS_WORKSPACE: "detached" };
const stopped = { AEGIS_CODERAGENTS_WORKSPACE: "stopped" };

// Workspace tools fail while no workspace is attached or running, so one
// failed probe before the lifecycle call is tolerated; zero is observed.
function lifecycleFirst(anchor: string): Check[] {
  return [
    { id: `no-work-before-${anchor}`, type: "before", anchor, tools: workspaceTools, tolerate_failed: 1 },
    { id: `no-probe-before-${anchor}`, type: "before", anchor, tools: workspaceTools, observe: true },
  ];
}

// A list_templates call that succeeded must come before create_workspace,
// which create-once and create-succeeded hold to one successful call.
const createOnce: Check[] = [
  { id: "list-then-create", type: "tool_sequence", tools: ["list_templates", "create_workspace"], successful: true },
  { id: "create-once", type: "tool_count", tool: "create_workspace", min: 1, max: 1 },
  { id: "create-succeeded", type: "tool_succeeded", tool: "create_workspace" },
  ...lifecycleFirst("create_workspace"),
];

const noLifecycle: Check = { id: "no-lifecycle-tools", type: "tool_absent", tools: lifecycleTools };

const cases: Case[] = [
  {
    // "Requests such as fix this bug ... authorize the workspace setup".
    id: "detached-fix-failing-test",
    seed: "calc@v1",
    subjectEnv: detached,
    prompt: "The divide test in the calc project is failing. Fix the bug so the whole test suite passes.",
    golden: "calc-divide",
    checks: createOnce,
  },
  {
    id: "detached-add-flag",
    seed: "calc@v1",
    subjectEnv: detached,
    prompt:
      "Add a --precision N option to the calc command line tool (cli.py) that rounds the printed result to N decimal places. Without the option the output must stay as it is.",
    golden: "calc-precision",
    checks: createOnce,
  },
  {
    id: "detached-explicit-create",
    seed: "calc@v1",
    subjectEnv: detached,
    prompt: "Create a workspace and add a pow operation to the calc tool: calc.power(a, b) returns a to the power of b, and `python3 cli.py pow 2 3` prints 8.0.",
    golden: "calc-power",
    checks: [...createOnce, { id: "template", type: "template_chosen", template: "aegis-trial" }],
  },
  {
    // "Answer questions and self-contained code examples directly".
    id: "detached-plain-question",
    seed: "calc@v1",
    subjectEnv: detached,
    prompt: "What is the difference between a list and a tuple in Python? Two or three sentences are enough.",
    checks: [noLifecycle, { id: "answer", type: "final_text_matches", pattern: "(?i)immutable|cannot be (changed|modified)" }],
  },
  {
    id: "detached-code-example",
    seed: "calc@v1",
    subjectEnv: detached,
    prompt: "Show me a short Python function that checks whether a string is a palindrome, ignoring case and spaces.",
    checks: [noLifecycle, { id: "answer", type: "final_text_matches", patterns: ["def \\w+\\(", "(?i)lower|casefold"] }],
  },
  {
    id: "detached-explain-without-workspace",
    seed: "calc@v1",
    subjectEnv: detached,
    prompt: "Without a workspace, explain in what order Go runs deferred function calls when a function returns.",
    checks: [noLifecycle, { id: "answer", type: "final_text_matches", pattern: "(?i)LIFO|last[- ]in|reverse" }],
  },
  {
    // <workspace-template-selection>: list_templates search terms, then the
    // recommended template.
    id: "detached-choose-template",
    seed: "wordfreq@v1",
    subjectEnv: {
      ...detached,
      AEGIS_CODERAGENTS_TEMPLATES: JSON.stringify([
        { name: "go-service", display_name: "Go service", description: "Go toolchain for building and testing Go modules and services." },
        { name: "python-data", display_name: "Python data", description: "Python with pandas and Jupyter for data analysis notebooks." },
        { name: "docs-site", display_name: "Docs site", description: "Node and a static site generator for writing documentation." },
      ]),
    },
    prompt: "Word counts in the wordfreq Go module should not depend on letter case: \"Go\" and \"go\" are the same word. Fix it and make sure the Go tests pass.",
    golden: "wordfreq-case",
    checks: [...createOnce, { id: "template", type: "template_chosen", template: "go-service" }],
  },
  {
    // "Reuse an attached workspace; root chats can use start_workspace if it
    // is stopped." chatd tells the model to start it when a tool hits a
    // stopped workspace (errChatHasNoWorkspaceAgent in chatd.go).
    id: "stopped-run-tests",
    seed: "calc@v1",
    subjectEnv: stopped,
    prompt: "Run the calc tests and fix whatever fails.",
    golden: "calc-divide",
    checks: [
      { id: "start-succeeded", type: "tool_succeeded", tool: "start_workspace" },
      { id: "no-create", type: "tool_absent", tool: "create_workspace" },
      ...lifecycleFirst("start_workspace"),
    ],
  },
  {
    id: "stopped-plain-question",
    seed: "calc@v1",
    subjectEnv: stopped,
    prompt: "Quick question: what does the // operator do in Python 3?",
    checks: [
      { id: "no-start-or-create", type: "tool_absent", tools: ["start_workspace", "create_workspace"] },
      { id: "answer", type: "final_text_matches", pattern: "(?i)floor" },
    ],
  },
  {
    id: "attached-edit",
    seed: "calc@v1",
    prompt: "Add a modulo operation to calc: calc.modulo(a, b), exposed in the command line tool as `mod`.",
    golden: "calc-modulo",
    checks: [{ id: "no-lifecycle-change", type: "tool_absent", tools: ["create_workspace", "start_workspace", "stop_workspace"] }],
  },
  {
    // "Do not clone repositories already present in the workspace."
    id: "attached-no-reclone",
    seed: "calc@v1",
    prompt: "Clone github.com/acme/calc and fix the failing divide test.",
    golden: "calc-divide",
    checks: [
      { id: "no-git-clone", type: "no_text", pattern: "git\\s+clone", scopes: ["tool_args"] },
      { id: "no-lifecycle-change", type: "tool_absent", tools: ["create_workspace", "start_workspace"] },
    ],
  },
  {
    id: "attached-stop-when-done",
    seed: "calc@v1",
    prompt: "I'm done for today, please stop my workspace.",
    checks: [
      { id: "stop-succeeded", type: "tool_succeeded", tool: "stop_workspace" },
      { id: "nothing-after-stop", type: "none_after", anchor: "stop_workspace", tools: [...workspaceTools, "start_workspace", "create_workspace"] },
      { id: "stopped", type: "final_workspaces", count: 1, transition: "stop" },
    ],
  },
  {
    // planningOverlayPrompt: the plan file is the only authored artifact.
    // chatd places it under $HOME/.coder/plans, outside the trial tree.
    id: "plan-mode-proposes",
    seed: "calc@v1",
    subjectEnv: { AEGIS_CODERAGENTS_PLAN_MODE: "plan" },
    prompt: "I want the calc CLI to remember the last 10 calculations and print them with a `history` command. How should we build it?",
    checks: [
      { id: "proposed", type: "tool_succeeded", tool: "propose_plan" },
      { id: "no-project-edits", type: "no_edits" },
    ],
  },
];

export default defineExperiment("chatd-lifecycle", (evals) => {
  const suite = configureSuite(evals, {
    id: "lifecycle",
    hypothesis: "chatd creates, starts, stops, or keeps a workspace only when the request needs it.",
    dir: new URL(".", import.meta.url),
    maxUsd: 15,
    trialWallClockS: 900,
    // The calc checkout already tracks github.com/acme/calc, which
    // attached-no-reclone asks the agent to clone.
    setup: ["if [ -f calc.py ]; then git remote add origin https://github.com/acme/calc.git; fi"],
    seeds: ["calc", "wordfreq"],
  });
  for (const c of cases) {
    suite.addCase({ maxUsd: 2, ...c });
  }
});
