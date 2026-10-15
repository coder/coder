"use strict";

const rules = require("./migration-numbers.cjs");

/**
 * Trusted orchestration. GitHub trees and blobs are data only; this module
 * never checks out a PR, runs its scripts, or evaluates its SQL.
 */
async function run({ github, context, core, writer }) {
  const { owner, repo } = context.repo;
  const repository = owner + "/" + repo;
  const cache = new Map();
  const mainSha = async () => (await github.rest.repos.getBranch({ owner, repo, branch: "main" })).data.commit.sha;
  const getPR = async (number) => (await github.rest.pulls.get({ owner, repo, pull_number: number })).data;
  const snapshot = async (sha) => {
    if (!cache.has(sha)) {
      const { data } = await github.rest.git.getTree({ owner, repo, tree_sha: sha, recursive: "1" });
      if (data.truncated) throw new Error("Git tree is truncated; cannot safely check migration numbering.");
      cache.set(sha, Object.fromEntries(data.tree.filter((entry) => entry.type !== "tree")
        .map((entry) => [entry.path, entry])));
    }
    return cache.get(sha);
  };
  const blob = async (entry) => {
    if (entry.type !== "blob" || entry.mode !== "100644" || entry.size > 1024 * 1024) {
      throw new Error("Unsupported file type or oversized file; manual fix required.");
    }
    const { data } = await github.rest.git.getBlob({ owner, repo, file_sha: entry.sha });
    if (data.encoding !== "base64" || data.size > 1024 * 1024) throw new Error("Unsupported blob; manual fix required.");
    return Buffer.from(data.content, "base64");
  };
  const allPRs = await github.paginate(github.rest.pulls.list, { owner, repo, state: "open", per_page: 100 });
  let targets;
  if (context.eventName === "push" || context.eventName === "workflow_dispatch") {
    targets = allPRs.filter((pr) => pr.base.ref === "main").map((pr) => pr.number);
  } else {
    targets = [context.payload.pull_request.number];
  }

  async function publish(pr, main, result, detail) {
    const current = await getPR(pr.number);
    if (!rules.sameRevision(pr, current, main, await mainSha())) {
      core.info("Discarding stale result for PR #" + pr.number + ".");
      return;
    }
    await github.rest.repos.createCommitStatus({ owner, repo, sha: pr.head.sha, context: rules.CONTEXT,
      state: result.errors.length ? "failure" : "success",
      description: (detail || result.errors[0] || "Migration numbers are current.").replace(/[\r\n]/g, " ").slice(0, 140),
      target_url: context.serverUrl + "/" + repository + "/actions/runs/" + context.runId });
    core.info("PR #" + pr.number + ": " + (detail || result.errors.join("; ") || "Migration numbers are current."));
    for (const rename of result.renames) core.info("Rename " + rename.from + " to " + rename.to);
  }

  for (const number of targets) {
    const pr = await getPR(number);
    if (pr.state !== "open" || pr.base.ref !== "main") continue;

    const main = await mainSha();
    try {
      const { data: comparison } = await github.rest.repos.compareCommitsWithBasehead({
        owner, repo, basehead: main + "..." + pr.head.sha });
      const base = await snapshot(comparison.merge_base_commit.sha);
      const head = await snapshot(pr.head.sha);
      const result = rules.plan(base, await snapshot(main), head);
      if (!result.affected || !result.errors.length) {
        await publish(pr, main, result);
        continue;
      }
      const isStacked = (pulls) => pulls.some((other) => other.number !== number
        && other.head.repo?.full_name === repository
        && (other.base.ref === pr.head.ref || comparison.commits.some((commit) => commit.sha === other.head.sha)));
      const stacked = isStacked(allPRs);
      let reason = rules.skipReason(pr, repository, stacked);
      if (!reason && comparison.total_commits > comparison.commits.length) reason = "Incomplete commit history; manual fix required.";
      if (!reason && !result.repairable) reason = "Unsafe migration change; fix the reported errors manually.";
      if (!reason && !writer) {
        reason = "Autofix needs BACKPORT_APP_ID and BACKPORT_APP_PRIVATE_KEY with contents:write access.";
      }
      const contents = new Map();
      if (!reason) {
        await publish(pr, main, result, "Migration numbers conflict; checking automatic repair.");
        // Read all changed text, including SQL and test references. No PR blob
        // is ever executed. Deletions cannot contain a surviving reference.
        const texts = {};
        for (const path of result.changes) {
          if (!head[path]) continue;
          const data = await blob(head[path]);
          contents.set(path, data);
          if (data.includes(0)) {
            reason = "Binary PR changes require a manual migration fix.";
            break;
          }
          texts[path] = data.toString("utf8");
        }
        reason ||= rules.referenceReason(result, texts);
      }
      if (reason) {
        await publish(pr, main, result, "No automatic edit: " + reason);
        continue;
      }

      // Re-read both refs and eligibility immediately before mutation. GitHub
      // atomically rejects an author push via expectedHeadOid. Main/label reads
      // are best-effort; branch protection remains the final freshness gate.
      const current = await getPR(number);
      const currentPRs = await github.paginate(github.rest.pulls.list, { owner, repo, state: "open", per_page: 100 });
      if (!rules.unchanged(pr, current, main, await mainSha()) || rules.skipReason(current, repository, isStacked(currentPRs))) {
        core.info("Discarding stale autofix for PR #" + number + ".");
        continue;
      }
      const destinations = new Set(result.renames.map((rename) => rename.to));
      const input = {
        branch: { repositoryNameWithOwner: repository, branchName: pr.head.ref },
        expectedHeadOid: pr.head.sha,
        message: { headline: "fix(coderd/database/migrations): renumber new migrations" },
        fileChanges: {
          deletions: result.renames.filter((rename) => !destinations.has(rename.from)).map((rename) => ({ path: rename.from })),
          additions: result.renames.map((rename) => ({ path: rename.to, contents: contents.get(rename.from).toString("base64") })),
        },
      };
      // writer holds the App token and is used only for this call. It appends a
      // normal signed commit and triggers ordinary CI, unlike GITHUB_TOKEN commits.
      const response = await writer.graphql(
        "mutation($input: CreateCommitOnBranchInput!) { createCommitOnBranch(input: $input) { commit { oid } } }",
        { input });
      core.info("Renumbered migrations for PR #" + number + " in " + response.createCommitOnBranch.commit.oid + ".");
      // Do not mark the old head green. The App commit starts normal CI and a
      // synchronize event evaluates the new head against current main.
    } catch (error) {
      core.warning("PR #" + number + ": " + error.message);
      await publish(pr, main, { errors: [error.message], renames: [] }, "No automatic edit: " + error.message);
    }
  }
}

module.exports = { run };
