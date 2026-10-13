import { existsSync, readFileSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import { defineExperiment } from "@coder/aegis";
import { configureSuite, graderBinary } from "../shared.ts";

// Terminal-Bench 2.1 subset with a hybrid verifier: the agent works on a
// copy of the task image's /app inside the aegis runner image, and the
// trusted grader reruns the task's own tests/test.sh in the task image
// against the agent's final tree. run.sh imports the tasks with
// `chatd-eval-grader tb-import`; without an import (for example plain
// `aegis validate`) the tasks compile against placeholder seeds.

interface TaskList {
  source: { commit: string };
  tasks: Array<{ id: string }>;
}

interface Seeds {
  tasks: Record<string, { seed_repo: string; seed_ref: string; instruction: string }>;
}

const list = JSON.parse(readFileSync(new URL("tasks.json", import.meta.url), "utf8")) as TaskList;
const cache = process.env.CHATD_EVAL_CACHE || join(homedir(), ".cache", "chatd-evals");
const tbDir = process.env.CHATD_EVAL_TB_DIR || join(cache, "terminal-bench", `import-${list.source.commit.slice(0, 7)}`);
const seedsFile = join(tbDir, "seeds.json");
const seeds: Seeds = existsSync(seedsFile) ? JSON.parse(readFileSync(seedsFile, "utf8")) : { tasks: {} };

const cwdNote =
  "\n\nNote: your current working directory holds this task's files and stands in for /app. Wherever these instructions say /app, use the current working directory instead.";

export default defineExperiment("chatd-terminal-bench", (evals) => {
  const suite = configureSuite(evals, {
    id: "terminalbench",
    hypothesis: "chatd solves a curated Terminal-Bench 2.1 subset, verified by each task's own tests.",
    dir: new URL(".", import.meta.url),
    maxUsd: 40,
    trialWallClockS: 2700,
    agentMemoryMiB: 4096,
  });
  const verifier = evals.customGrader("terminal_bench", {
    type: "subprocess",
    binary: graderBinary,
    // Covers pulling the task image and the verifier's own tool installs.
    timeout_s: 3600,
    params: ["task"],
    args: { mode: "terminal_bench", tb_dir: tbDir, task: "{{task}}" },
  });
  for (const { id } of list.tasks) {
    if (!suite.wanted(id)) {
      continue;
    }
    const seed = seeds.tasks[id];
    evals.baseRepo(`${id}@tb`, {
      git: seed
        ? { repo: seed.seed_repo, ref: seed.seed_ref }
        : { repo: `file://${join(tbDir, "tasks", id, "seed.git")}`, ref: "0".repeat(40) },
    });
    evals.test(id, { baseRepo: `${id}@tb`, skillApplicable: "applicable", tags: ["terminal-bench"] }, (t) => {
      t.prompt((seed?.instruction ?? `Terminal-Bench task ${id} (not imported).`).trim() + cwdNote);
      t.expect(verifier.use({ task: id }));
      t.expect(suite.guardrails([{ id: "max-tool-errors", type: "max_tool_errors", max: 10 }]));
      t.cost.atMost(8);
    });
  }
});
