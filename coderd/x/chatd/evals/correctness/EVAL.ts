import { defineExperiment } from "@coder/aegis";
import { configureSuite, type Case } from "../shared.ts";

// Correctness: golden coding tasks in an attached workspace. Hidden tests
// decide task success; grader/golden_test.go proves each one fails on its
// seed and passes with the reference solution under testdata/golden.

const guardrails = [
  { id: "max-tool-errors", type: "max_tool_errors", max: 5 },
  // Matched against each command an execute call runs, from its start: a
  // test runner, or a Python or Node test file run directly.
  { id: "tested-after-edit", type: "test_after_edit", pattern: "^(?:python[0-9.]* (?:-[WX] \\S+ |-\\S+ )*(?:-m (?:unittest|pytest)\\b|(?:\\S*/)?test_\\w*\\.py(?:\\s|$))|py\\.?test\\b|go test\\b|node (?:-\\S+ )*(?:--test\\b|(?:\\S*/)?[\\w.-]+\\.test\\.[cm]?js(?:\\s|$))|npm (?:run )?test\\b)" },
];

const cases: Case[] = [
  {
    id: "py-bugfix-durations",
    seed: "durations@v1",
    prompt: "parse_duration(\"1h30m\") returns 1800 instead of 5400. Fix the bug.",
    golden: "durations-combined",
  },
  {
    id: "py-debug-from-log",
    seed: "invoices@v1",
    prompt: "Last night's invoice import crashed; the log is in issue.log. Find the cause and fix it.",
    golden: "invoices-legacy-currency",
  },
  {
    id: "go-bugfix-lru",
    seed: "lru@v1",
    prompt: "Our LRU cache evicts entries that were just read with Get. Reading an entry should count as using it. Fix it.",
    golden: "lru-recency",
  },
  {
    id: "go-multi-file-tebibytes",
    seed: "units@v1",
    prompt: "Add TiB support to the units package: ParseSize should accept values like \"2TiB\" and FormatSize should use TiB for sizes of 1 TiB or more.",
    golden: "units-tebibytes",
  },
  {
    id: "node-feature-max-length",
    seed: "slug@v1",
    prompt:
      "Add an optional maxLength to slugify(text, { maxLength }): the slug is cut at the last dash that fits, or mid-word when even the first word is longer than maxLength.",
    golden: "slug-max-length",
  },
  {
    id: "node-refactor-shared-formatter",
    seed: "report@v1",
    prompt:
      "csvReport and tsvReport in report.js duplicate each other. Extract the shared logic into an exported formatTable(rows, separator) and make both use it, without changing their output.",
    golden: "report-shared-formatter",
  },
];

export default defineExperiment("chatd-correctness", (evals) => {
  const suite = configureSuite(evals, {
    id: "correctness",
    hypothesis: "chatd completes small coding tasks correctly, as judged by hidden tests.",
    dir: new URL(".", import.meta.url),
    maxUsd: 15,
    trialWallClockS: 1200,
    agentMemoryMiB: 4096,
    seeds: ["durations", "invoices", "lru", "units", "slug", "report"],
  });
  for (const c of cases) {
    suite.addCase({ maxUsd: 3, guardrails, ...c });
  }
});
