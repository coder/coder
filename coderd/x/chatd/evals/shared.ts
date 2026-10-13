// Shared configuration for the chatd eval suites. run.sh passes its inputs
// through CHATD_EVAL_* environment variables at compile time; with none set,
// every suite compiles against the runner image's released coder so
// `aegis validate` works standalone.
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import type { AuthoringGraderRef, ExperimentBuilder, TestBuilder } from "@coder/aegis";

const env = process.env;

/** Runner image content tag built from AEGIS_REF in run.sh. */
export const defaultRunnerImage = "aegis-runner:9666dceb487a";
/** coder release baked into the runner image. */
export const releaseVersion = "2.37.0";

export const model = env.CHATD_EVAL_MODEL || "anthropic:claude-sonnet-5-5";
export const image = env.CHATD_EVAL_IMAGE || defaultRunnerImage;
export const coderVersion = env.CHATD_EVAL_CODER_VERSION || releaseVersion;
export const customCoder = env.CHATD_EVAL_CUSTOM_CODER === "1";
// Placeholder when unset: validation does not execute subprocess graders.
export const graderBinary = env.CHATD_EVAL_GRADER || "/usr/local/bin/chatd-eval-grader";
export const repeats = positiveInt(env.CHATD_EVAL_REPEATS, 1);
const maxUsdOverride = positiveNumber(env.CHATD_EVAL_MAX_USD);
const promptFile = env.CHATD_EVAL_PROMPT_FILE || "";
const promptMode = env.CHATD_EVAL_PROMPT_MODE || "append";
const caseFilter = new Set(
  (env.CHATD_EVAL_CASES || "")
    .split(",")
    .map((s) => s.trim())
    .filter((s) => s !== ""),
);

/**
 * USD per million tokens, from coderd/aibridge/prices/data/prices.json.
 * Every model a trial may call needs an entry or aegis fails the trial.
 */
const pricing = {
  "claude-sonnet-5-5": { input: 2, output: 10, cacheRead: 0.1, cacheWrite: 2.5 },
  "claude-opus-5-5": { input: 4, output: 20, cacheRead: 0.2, cacheWrite: 5 },
  "claude-haiku-5-5": { input: 0.1, output: 0.5, cacheRead: 0.01, cacheWrite: 0.125 },
};

export const controlArm = "control";
export const candidateArm = "candidate-prompt";

/** One entry of the grader's checks DSL (see grader/checks.go). */
export interface Check {
  id: string;
  type: string;
  observe?: boolean;
  tool?: string;
  tools?: string[];
  anchor?: string;
  tolerate_failed?: number;
  min?: number;
  max?: number;
  successful?: boolean;
  pattern?: string;
  patterns?: string[];
  after?: string;
  background?: boolean;
  scopes?: string[];
  template?: string;
  count?: number;
  transition?: string;
  allow?: string[];
  deny?: string[];
  require?: string[];
  branches?: string[];
  max_limit?: number;
}

export interface Case {
  id: string;
  /** Base repo id registered with evals.baseRepo. */
  seed: string;
  prompt: string;
  subjectEnv?: Record<string, string>;
  /** Required and observe-only behavior checks; they count toward task success. */
  checks?: Check[];
  /** Checks reported as guardrails; they never decide task success. */
  guardrails?: Check[];
  /** Directory under <suite>/testdata/golden whose hidden tests must pass. */
  golden?: string;
  maxUsd?: number;
  wallClockS?: number;
  tags?: string[];
}

export interface SuiteOptions {
  id: string;
  hypothesis: string;
  /** Directory of the suite's EVAL.ts, as new URL(".", import.meta.url). */
  dir: URL;
  maxUsd: number;
  trialWallClockS: number;
  agentMemoryMiB?: number;
  /**
   * Suite setup commands. They run in every trial workspace after aegis
   * commits the eval-baseline and again when grading rehydrates it, so they
   * may only touch git metadata or ignored files.
   */
  setup?: string[];
  /** Seed directories under <suite>/testdata/seeds, registered as <name>@v1. */
  seeds?: string[];
}

export interface SuiteHandles {
  checks: (list: Check[]) => AuthoringGraderRef;
  guardrails: (list: Check[]) => AuthoringGraderRef;
  hidden: (golden: string) => AuthoringGraderRef;
  addCase: (c: Case) => void;
  wanted: (id: string) => boolean;
}

/**
 * Configures an experiment with the shared subject, pricing, budget,
 * variants, and graders, and returns helpers that register cases.
 */
export function configureSuite(evals: ExperimentBuilder, opts: SuiteOptions): SuiteHandles {
  const subjectEnv: Record<string, string> = {};
  if (customCoder) {
    subjectEnv.AEGIS_CODERAGENTS_CUSTOM_CODER = "1";
  }
  evals.configure({
    backend: "docker",
    backendOptions: { docker: definedOnly({ image, agentMemoryMiB: opts.agentMemoryMiB }) },
    hypothesis: opts.hypothesis,
    subject: {
      harness: "coder-agents",
      harnessVersion: coderVersion,
      controlTransport: "chat-rest",
      model,
      allowedTools: [],
      env: subjectEnv,
    },
    pricing,
    repeats,
    trialWallClockS: opts.trialWallClockS,
    budget: { maxUsd: maxUsdOverride ?? opts.maxUsd, confirmRequired: false, pilotMaxUsd: 0 },
    primaryMetric: "task_success",
    efficacyMetrics: ["task_success"],
    guardrailMetrics: [],
    scheduling: { order: "interleaved-random", retry: "infra-only" },
  });

  const checksGrader = evals.customGrader("checks", {
    type: "subprocess",
    binary: graderBinary,
    timeout_s: 120,
    params: ["checks"],
    args: { mode: "checks", checks: "{{checks}}" },
  });
  const guardrailGrader = evals.customGrader("guardrails", {
    type: "subprocess",
    binary: graderBinary,
    timeout_s: 120,
    guardrail: true,
    params: ["checks"],
    args: { mode: "checks", checks: "{{checks}}" },
  });
  // Hidden tests run as a command grader in the rehydrated workspace, which
  // run.sh executes only inside the isolated grading container. The grader
  // environment has no HOME, so every toolchain cache points at /tmp.
  const hiddenGrader = evals.customGrader("hidden_tests", {
    type: "command",
    run: "{{script}}",
    timeout_s: 600,
    params: ["script"],
    env: {
      GOCACHE: "/tmp/chatd-eval-go/cache",
      GOPATH: "/tmp/chatd-eval-go/path",
      GOMODCACHE: "/tmp/chatd-eval-go/mod",
      GOFLAGS: "-mod=mod",
      GOPROXY: "off",
      GOTOOLCHAIN: "local",
      GOTELEMETRY: "off",
      XDG_CACHE_HOME: "/tmp/chatd-eval-xdg",
      PYTHONDONTWRITEBYTECODE: "1",
      NO_COLOR: "1",
    },
  });

  evals.suite(opts.id, { setup: opts.setup ?? [], defaultGraders: [] });
  for (const seed of opts.seeds ?? []) {
    evals.baseRepo(`${seed}@v1`, { dir: `testdata/seeds/${seed}` });
  }

  evals.variant(controlArm, () => {});
  const contrasts: Array<readonly [string, string]> = [];
  if (promptFile !== "") {
    const prompt = readFileSync(promptFile, "utf8");
    if (promptMode !== "append" && promptMode !== "replace") {
      throw new Error(`CHATD_EVAL_PROMPT_MODE must be append or replace, got ${promptMode}`);
    }
    evals.variant(candidateArm, (v) => {
      v.harnessFlag("AEGIS_CODERAGENTS_SYSTEM_PROMPT", prompt);
      v.harnessFlag("AEGIS_CODERAGENTS_INCLUDE_DEFAULT_SYSTEM_PROMPT", promptMode === "append" ? "true" : "false");
    });
    contrasts.push([controlArm, candidateArm]);
  }
  evals.analysis({ pairing: "by_task", perArmCI: "wilson", significance: "paired_bootstrap", contrasts });

  const checks = (list: Check[]) => checksGrader.use({ checks: JSON.stringify(list) });
  const guardrails = (list: Check[]) => guardrailGrader.use({ checks: JSON.stringify(list) });
  const hidden = (golden: string) => hiddenGrader.use({ script: hiddenTestScript(new URL(`testdata/golden/${golden}/`, opts.dir)) });
  const wanted = (id: string) => caseFilter.size === 0 || caseFilter.has(id);

  const addCase = (c: Case) => {
    if (!wanted(c.id)) {
      return;
    }
    evals.test(
      c.id,
      definedOnly({
        baseRepo: c.seed,
        skillApplicable: "applicable" as const,
        subjectEnv: c.subjectEnv,
        wallClockS: c.wallClockS,
        tags: c.tags,
      }),
      (t: TestBuilder) => {
        t.prompt(c.prompt);
        if (c.golden !== undefined) {
          t.expect(hidden(c.golden));
        }
        if (c.checks !== undefined && c.checks.length > 0) {
          t.expect(checks(c.checks));
        }
        if (c.guardrails !== undefined && c.guardrails.length > 0) {
          t.expect(guardrails(c.guardrails));
        }
        if (c.maxUsd !== undefined) {
          t.cost.atMost(c.maxUsd);
        }
      },
    );
  };
  return { checks, guardrails, hidden, addCase, wanted };
}

/**
 * Builds the hidden-test command for a golden case: it writes the files
 * under hidden/ into the graded workspace and runs golden.json's command.
 * The files travel inside the compiled bundle, so they never enter the
 * agent's checkout and changing them changes the bundle digest.
 */
export function hiddenTestScript(goldenDir: URL): string {
  const root = fileURLToPath(goldenDir);
  const golden = JSON.parse(readFileSync(join(root, "golden.json"), "utf8")) as { command: string };
  if (typeof golden.command !== "string" || golden.command === "") {
    throw new Error(`${root}golden.json: command is required`);
  }
  const lines = ["set -e"];
  const hiddenDir = join(root, "hidden");
  for (const file of listFiles(hiddenDir)) {
    const rel = relative(hiddenDir, file).split(sep).join("/");
    if (!/^[A-Za-z0-9_./-]+$/.test(rel) || rel.split("/").includes("..")) {
      throw new Error(`hidden file ${rel}: unsupported path`);
    }
    const parent = rel.includes("/") ? rel.slice(0, rel.lastIndexOf("/")) : ".";
    const b64 = readFileSync(file).toString("base64");
    lines.push(`mkdir -p '${parent}'`, `printf '%s' '${b64}' | base64 -d > '${rel}'`);
  }
  lines.push(golden.command);
  return lines.join("\n");
}

function listFiles(dir: string): string[] {
  if (!existsSync(dir)) {
    return [];
  }
  const out: string[] = [];
  for (const name of readdirSync(dir).sort()) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) {
      out.push(...listFiles(full));
    } else {
      out.push(full);
    }
  }
  return out;
}

// definedOnly drops undefined fields, which the aegis compiler rejects.
function definedOnly<T extends object>(value: T): T {
  return Object.fromEntries(Object.entries(value).filter(([, v]) => v !== undefined)) as T;
}

function positiveInt(value: string | undefined, fallback: number): number {
  if (value === undefined || value === "") {
    return fallback;
  }
  const n = Number(value);
  if (!Number.isInteger(n) || n < 1) {
    throw new Error(`expected a positive integer, got ${value}`);
  }
  return n;
}

function positiveNumber(value: string | undefined): number | undefined {
  if (value === undefined || value === "") {
    return undefined;
  }
  const n = Number(value);
  if (!Number.isFinite(n) || n <= 0) {
    throw new Error(`expected a positive number, got ${value}`);
  }
  return n;
}

/** Tools that act on a workspace and fail while none is attached or running. */
export const workspaceTools = [
  "execute",
  "process_output",
  "process_list",
  "process_signal",
  "read_file",
  "write_file",
  "edit_files",
  "attach_file",
];

/** Tools that change workspace lifecycle. */
export const lifecycleTools = ["list_templates", "read_template", "create_workspace", "start_workspace", "stop_workspace"];
