import { defineExperiment } from "@coder/aegis";
import { configureSuite, type Case } from "../shared.ts";

// Tool descriptions: tasks that lean on one tool's contract each. The
// primary metric is the per-tool failure rate in the summary, compared
// across builds; every case also carries a max_tool_errors guardrail.

const toolErrors = (tool?: string, max = 1) => ({
  id: tool ? `max-${tool.replace(/_/g, "-")}-errors` : "max-tool-errors",
  type: "max_tool_errors",
  ...(tool ? { tool } : {}),
  max,
});

const cases: Case[] = [
  {
    // edit_files: tab-indented Go, space-indented Python, and YAML, each with
    // a second identical block that must not change.
    id: "edit-files-precise",
    seed: "settings@v1",
    prompt:
      "Raise the default timeout from 30 to 45 seconds in settings/config.go, settings/defaults.py, and settings/defaults.yaml. The fallback profile must keep 30.",
    golden: "settings-timeout",
    guardrails: [toolErrors("edit_files"), toolErrors(undefined, 3)],
  },
  {
    // read_file paging: normalize_name sits near line 2360 of a 2900-line
    // file; copying read_file line-number prefixes into edit_files search
    // strings shows up as edit_files failures.
    id: "read-file-deep-function",
    seed: "records@v1",
    prompt: "normalize_name in records/normalize.py should turn empty or whitespace-only names into \"unknown\" and collapse repeated spaces. Fix it.",
    golden: "records-name",
    guardrails: [toolErrors("edit_files"), toolErrors("read_file"), toolErrors(undefined, 3)],
  },
  {
    id: "write-file-nested-path",
    seed: "records@v1",
    prompt:
      "Add records/exporters/spreadsheet/writer.py with write_rows(rows): it takes a list of dicts and returns CSV text whose header comes from the first row's keys, or an empty string for no rows.",
    golden: "records-csv-writer",
    checks: [{ id: "write-file-used", type: "tool_succeeded", tool: "write_file" }],
    guardrails: [toolErrors("write_file"), toolErrors(undefined, 3)],
  },
  {
    // execute reports exit_code; a non-zero exit is the answer, not a tool
    // failure to retry around.
    id: "execute-exit-code",
    seed: "worker@v1",
    prompt: "Does `python3 check.py` pass in this repo? Tell me the exit code it returns and why.",
    checks: [
      { id: "ran-check", type: "execute_matches", pattern: "check\\.py" },
      { id: "exit-code", type: "final_text_matches", patterns: ["\\b3\\b", "(?i)queue_url"] },
      { id: "no-edits", type: "no_edits" },
    ],
    guardrails: [toolErrors(undefined, 2)],
  },
  {
    // execute run_in_background, then process_output and process_signal on
    // the returned process id.
    id: "background-worker",
    seed: "worker@v1",
    prompt: "Start worker.py in the background, wait until it has processed at least 3 jobs, show me its output, then stop it.",
    checks: [
      { id: "started-in-background", type: "execute_matches", pattern: "worker\\.py", background: true },
      { id: "read-output", type: "tool_succeeded", tool: "process_output" },
      { id: "stopped", type: "tool_succeeded", tool: "process_signal" },
      { id: "output-shown", type: "final_text_matches", pattern: "processed job [3-9]" },
    ],
    guardrails: [toolErrors("process_output"), toolErrors("process_signal"), toolErrors(undefined, 3)],
  },
  {
    // create_workspace: chatd turns an invalid requested name into a valid
    // generated one (generatedWorkspaceName in chattool/createworkspace.go).
    id: "create-workspace-invalid-name",
    seed: "records@v1",
    subjectEnv: { AEGIS_CODERAGENTS_WORKSPACE: "detached" },
    prompt: "Create a workspace called \"My Calc Env\" for this project and tell me the name it ended up with.",
    checks: [
      { id: "created", type: "tool_succeeded", tool: "create_workspace" },
      { id: "named", type: "final_text_matches", pattern: "(?i)my-?calc-?env" },
    ],
    guardrails: [toolErrors("create_workspace"), toolErrors(undefined, 3)],
  },
];

export default defineExperiment("chatd-tool-descriptions", (evals) => {
  const suite = configureSuite(evals, {
    id: "tooldescriptions",
    hypothesis: "chatd calls each tool within its described contract, with few failed calls.",
    dir: new URL(".", import.meta.url),
    maxUsd: 15,
    trialWallClockS: 900,
    seeds: ["settings", "records", "worker"],
  });
  for (const c of cases) {
    suite.addCase({ maxUsd: 2, ...c });
  }
});
