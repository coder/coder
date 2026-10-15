"use strict";

const assert = require("node:assert/strict");
const test = require("node:test");
const { execFileSync } = require("node:child_process");
const rules = require("./migration-numbers.cjs");
const { run } = require("./migration-automation.cjs");

function pair(version, stem) {
  const name = String(version).padStart(6, "0") + "_" + stem;
  return Object.fromEntries(["up", "down"].map((direction) => {
    const path = rules.MIGRATIONS + name + "." + direction + ".sql";
    return [path, { sha: path, mode: "100644", type: "blob", size: 8 }];
  }));
}
const base = pair(1, "initial");
const main = { ...base, ...pair(2, "taken") };
const added = pair(2, "ours");
const fixture = rules.FIXTURES + "000002_pre_ours.up.sql";
const head = { ...base, ...added, [fixture]: { sha: fixture, mode: "100644", type: "blob", size: 8 } };

test("fresh-main conflict produces a complete pair and fixture rename", () => {
  const result = rules.plan(base, main, head);
  assert.equal(result.repairable, true);
  assert.equal(result.renames.length, 3);
  assert.deepEqual(result.renames.map(({ to }) => to), [
    rules.MIGRATIONS + "000003_ours.up.sql",
    rules.MIGRATIONS + "000003_ours.down.sql",
    rules.FIXTURES + "000003_pre_ours.up.sql",
  ]);
});

test("multiple new migrations retain numeric order, including overlapping rename paths", () => {
  const result = rules.plan(base, main, { ...head, ...pair(3, "later") });
  assert.equal(result.repairable, true);
  assert.equal(result.renames.find(({ from }) => from.includes("later.up")).to, rules.MIGRATIONS + "000004_later.up.sql");
});

test("a collision after rebasing is repairable without moving published files", () => {
  const result = rules.plan(main, main, { ...main, ...added, [fixture]: head[fixture] });
  assert.equal(result.repairable, true);
  assert.match(result.errors.join("\n"), /Duplicate migration/);
  assert.ok(result.renames.every(({ from }) => !from.includes("taken")));
});

test("correct numbering and unrelated PRs explicitly succeed", () => {
  assert.deepEqual(rules.plan(base, base, { ...base, ...added }).errors, []);
  assert.equal(rules.plan(base, main, { ...base, "README.md": { sha: "new" } }).affected, false);
  assert.deepEqual(rules.plan(base, main, { ...base, "README.md": { sha: "new" } }).errors, []);
});

test("missing partners, mismatched stems and duplicate newly added pairs refuse repair", () => {
  for (const broken of [
    { ...base, [Object.keys(added)[0]]: Object.values(added)[0] },
    { ...base, [Object.keys(added)[0]]: Object.values(added)[0], [rules.MIGRATIONS + "000002_other.down.sql"]: Object.values(added)[1] },
    { ...base, ...added, ...pair(2, "another") },
  ]) {
    const result = rules.plan(base, main, broken);
    assert.ok(result.errors.length);
    assert.equal(result.repairable, false);
  }
});

test("historical edits can pass checks but cannot be included in automatic repair", () => {
  const modified = { ...base, [Object.keys(base)[0]]: { ...Object.values(base)[0], sha: "edited" } };
  assert.deepEqual(rules.plan(base, base, modified).errors, []);
  assert.equal(rules.plan(base, main, { ...modified, ...added }).repairable, false);
});

test("fixtures require an existing version and preserve legacy descriptions", () => {
  const invalid = { ...base, [fixture]: head[fixture] };
  assert.match(rules.validate(invalid).errors.join("\n"), /no matching migration/);
  const legacy = rules.FIXTURES + "000001_ initial_v0.6.6.up.sql";
  assert.deepEqual(rules.validate({ ...base, [legacy]: head[fixture] }).errors, []);
});

test("malformed paths, executable SQL and symbolic links fail closed", () => {
  for (const tree of [
    { ...base, [rules.MIGRATIONS + "oops.sql"]: head[fixture] },
    { ...base, [Object.keys(base)[0]]: { ...Object.values(base)[0], mode: "120000" } },
    { ...base, [Object.keys(base)[0]]: { ...Object.values(base)[0], mode: "100755" } },
  ]) assert.ok(rules.validate(tree).errors.length);
});

test("reference detection covers padded/unpadded constants, SQL and test filenames", () => {
  const result = rules.plan(base, main, head);
  for (const texts of [
    { "migrate_test.go": "migrateToVersion(2)" },
    { "migrate_test.go": 'ReadFile("000002_ours.up.sql")' },
    { "testdata/000002_reference.sql": "SELECT 1;" },
    { [fixture]: "-- applied after version 2" },
  ]) assert.match(rules.referenceReason(result, texts), /manual update/);
  assert.equal(rules.referenceReason(result, { "migrate_test.go": "value = 20" }), null);
});

function makePR() {
  return { number: 42, state: "open", base: { ref: "main" },
    head: { sha: "head", ref: "feature/migrations", repo: { full_name: "coder/coder" } },
    labels: [{ name: rules.LABEL }], mergeable: true, body: "" };
}


test("eligibility refuses forks, stacks, opt-out and merge conflicts", () => {
  for (const change of [
    (pr) => { pr.head.repo.full_name = "someone/coder"; },
    (pr) => { pr.labels = []; },
    (pr) => { pr.mergeable = false; },
    (pr) => { pr.mergeable = null; },
    (pr) => { pr.body = "https://graphite.dev/stack"; },
    (pr) => { pr.base.ref = "parent"; },
    (pr) => { pr.state = "closed"; },
  ]) {
    const pr = makePR();
    change(pr);
    assert.ok(rules.skipReason(pr, "coder/coder"));
  }
  assert.ok(rules.skipReason(makePR(), "coder/coder", true));
  assert.equal(rules.skipReason(makePR(), "coder/coder"), null);
});

test("status accepts mergeability/label metadata changes; writes reject them", () => {
  const pr = makePR();
  const next = structuredClone(pr);
  next.mergeable = null;
  next.labels.push({ name: "bug" });
  assert.equal(rules.sameRevision(pr, next, "main", "main"), true);
  assert.equal(rules.unchanged(pr, next, "main", "main"), false);
});

function harness(options = {}) {
  let pr = makePR();
  Object.assign(pr, options.pr || {});
  const statuses = [];
  const commits = [];
  const infos = [];
  const trees = { base, main, head: options.head || head };
  const texts = options.texts || {};
  let prReads = 0;
  let mainReads = 0;
  let lists = 0;
  const github = {
    rest: {
      pulls: {
        list: async () => ({ data: [pr, ...(lists++ && options.finalOthers ? options.finalOthers : options.others || [])] }),
        get: async () => {
          prReads++;
          if (options.mergeables) pr.mergeable = options.mergeables.length > 1 ? options.mergeables.shift() : options.mergeables[0];
          if (prReads === options.raceAt) options.race(pr);
          return { data: structuredClone(pr) };
        },
      },
      repos: {
        getBranch: async () => {
          mainReads++;
          return { data: { commit: { sha: mainReads === options.mainRaceAt ? "new-main" : "main" } } };
        },
        compareCommitsWithBasehead: async () => ({ data: {
          merge_base_commit: { sha: "base" }, total_commits: options.totalCommits || 1,
          commits: options.commitList || [{ sha: "head" }],
        } }),
        createCommitStatus: async (status) => { statuses.push(status); },
      },
      git: {
        getTree: async ({ tree_sha }) => ({ data: {
          truncated: options.truncated || false,
          tree: Object.entries(trees[tree_sha]).map(([path, entry]) => ({ ...entry, path })),
        } }),
        getBlob: async ({ file_sha }) => {
          const content = texts[file_sha] || "SELECT 1;";
          return { data: { content: Buffer.from(content).toString("base64"), encoding: "base64", size: content.length } };
        },
      },
    },
    paginate: async (method, params) => (await method(params)).data,
    graphql: async () => assert.fail("branch writes must use the App-token writer"),
  };
  const writer = {
    graphql: async (query, { input, ...rest }) => {
      assert.deepEqual(rest, {});
      assert.match(query, /createCommitOnBranch/);
      assert.equal(input.expectedHeadOid, "head");
      if (options.pushAtCommit) throw new Error("expectedHeadOid does not match");
      assert.equal(input.branch.repositoryNameWithOwner, "coder/coder");
      assert.equal(input.branch.branchName, "feature/migrations");
      assert.equal(input.fileChanges.additions.length, options.additionCount || 3);
      assert.equal(input.fileChanges.deletions.length, options.deletionCount || 3);
      const additions = new Set(input.fileChanges.additions.map(({ path }) => path));
      assert.ok(input.fileChanges.deletions.every(({ path }) => !additions.has(path)));
      for (const file of input.fileChanges.additions) assert.equal(Buffer.from(file.contents, "base64").toString(), "SELECT 1;");
      commits.push(input);
      return { createCommitOnBranch: { commit: { oid: "bot-head" } } };
    },
  };
  const context = {
    repo: { owner: "coder", repo: "coder" }, eventName: options.event || "pull_request_target",
    payload: { action: options.action || "synchronize", pull_request: { number: 42 } },
    serverUrl: "https://github.com", runId: 123,
  };
  const core = { info: (message) => infos.push(message), warning: (message) => infos.push(message) };
  const sleeps = [];
  const sleep = async (ms) => { sleeps.push(ms); };
  return { github, context, core, writer, sleep, sleeps, statuses, commits, infos, pr };
}

test("API repair preserves bytes and uses an atomic normal commit, never an old-head success", async () => {
  const h = harness();
  await run(h);
  assert.equal(h.commits.length, 1);
  assert.ok(h.statuses.length);
  assert.ok(h.statuses.every((status) => status.state === "failure" && status.context === rules.CONTEXT));
});

test("main migration push refreshes current PR heads rather than rerunning old CI", async () => {
  const h = harness({ event: "push" });
  await run({ ...h, writer: undefined });
  assert.equal(h.statuses[0].sha, "head");
  assert.match(h.statuses[0].description, /BACKPORT_APP_ID/);
  assert.equal(h.commits.length, 0);
});



test("missing credentials, forks, stacks, merge conflicts and hardcoded references never edit", async () => {
  for (const options of [
    { writer: undefined },
    { pr: { head: { sha: "head", ref: "feature/migrations", repo: { full_name: "someone/coder" } } } },
    { pr: { mergeable: false } },
    { others: [{ number: 43, base: { ref: "feature/migrations" }, head: { sha: "child", repo: { full_name: "coder/coder" } } }] },
    { head: { ...head, "migrate_test.go": { sha: "test", mode: "100644", type: "blob", size: 20 } },
      texts: { test: "migrateToVersion(2);" } },
  ]) {
    const h = harness(options);
    await run({ ...h, writer: Object.hasOwn(options, "writer") ? options.writer : h.writer });
    assert.equal(h.commits.length, 0);
    assert.equal(h.statuses.at(-1).state, "failure");
    assert.match(h.statuses.at(-1).description, /No automatic edit/);
  }
});

test("large generated and executable scanned files do not block repair", async () => {
  const queries = "// Code generated by sqlc. DO NOT EDIT.\n" + "x".repeat(1024 * 1024) + "\nversion = 2\n";
  const h = harness({ head: { ...head,
    "coderd/database/queries.sql.go": { sha: "queries", mode: "100644", type: "blob", size: queries.length },
    "scripts/check.sh": { sha: "script", mode: "100755", type: "blob", size: 9 } },
  texts: { queries } });
  await run(h);
  assert.equal(h.commits.length, 1);
  for (const header of ["// Package apidoc Code generated by swaggo/swag. DO NOT EDIT",
    "-- Code generated by 'make coderd/database/generate'. DO NOT EDIT."]) assert.ok(rules.generated(header + "\nversion 2"));
  assert.equal(rules.generated("package x\n// Code generated by sqlc. DO NOT EDIT."), false);
});

test("generated files still block repair when their filename has a moved number", async () => {
  const dump = "-- Code generated by 'make coderd/database/generate'. DO NOT EDIT.\n";
  const h = harness({ head: { ...head,
    "coderd/database/testdata/000002_dump.sql": { sha: "dump", mode: "100644", type: "blob", size: dump.length } },
  texts: { dump } });
  await run(h);
  assert.equal(h.commits.length, 0);
  assert.match(h.statuses.at(-1).description, /Migration number in filename/);
});

test("blob read errors name the file", async () => {
  for (const [entry, pattern] of [
    [{ sha: "large", mode: "100644", type: "blob", size: 101 * 1024 * 1024 }, /Oversized file/],
    [{ sha: "link", mode: "120000", type: "blob", size: 9 }, /Unsupported file type/],
  ]) {
    const h = harness({ head: { ...head, "coderd/large.go": entry } });
    await run(h);
    assert.equal(h.commits.length, 0);
    assert.match(h.statuses.at(-1).description, pattern);
    assert.match(h.statuses.at(-1).description, /coderd\/large\.go/);
  }
  const copied = { ...head, [fixture]: { ...head[fixture], size: 2 * 1024 * 1024 } };
  const h = harness({ head: copied });
  await run(h);
  assert.equal(h.commits.length, 0);
  assert.ok(h.statuses.at(-1).description.includes("Oversized file; manual fix required: " + fixture));
});

test("unknown mergeability waits before deciding on autofix", async () => {
  const h = harness({ mergeables: [null, true] });
  await run(h);
  assert.equal(h.commits.length, 1);
  assert.deepEqual(h.sleeps, [3000]);

  const final = harness({ mergeables: [true, true, null, true] });
  await run(final);
  assert.equal(final.commits.length, 1);
  assert.equal(final.sleeps.length, 1);

  const unknown = harness({ mergeables: [null] });
  await run(unknown);
  assert.equal(unknown.commits.length, 0);
  assert.equal(unknown.sleeps.length, 4);
  assert.match(unknown.statuses.at(-1).description, /unknown mergeability/);
});

test("author push, main movement and label removal before mutation discard autofix", async () => {
  for (const options of [
    { raceAt: 3, race: (pr) => { pr.head.sha = "author-head"; } },
    { mainRaceAt: 3 },
    { raceAt: 3, race: (pr) => { pr.labels = []; } },
  ]) {
    const h = harness(options);
    await run(h);
    assert.equal(h.commits.length, 0);
    assert.ok(h.infos.some((message) => message.includes("Discarding stale")));
  }
});

test("atomic commit rejection does not retry or overwrite the author", async () => {
  const h = harness({ pushAtCommit: true });
  await run(h);
  assert.equal(h.commits.length, 0);
  assert.ok(h.statuses.every((status) => status.state === "failure"));
});

test("a bot-renumbered head checks successfully without another commit", async () => {
  const repaired = { ...base, ...pair(3, "ours"), [rules.FIXTURES + "000003_pre_ours.up.sql"]: head[fixture] };
  const h = harness({ head: repaired });
  await run(h);
  assert.equal(h.commits.length, 0);
  assert.equal(h.statuses[0].state, "success");
});

test("truncated API trees fail closed with an explicit status", async () => {
  const h = harness({ truncated: true });
  await run(h);
  assert.equal(h.commits.length, 0);
  assert.equal(h.statuses[0].state, "failure");
  assert.match(h.statuses[0].description, /truncated/);
});

test("CLI validates the real repository tree and an unchanged PR", () => {
  for (const refs of [["HEAD"], ["HEAD", "HEAD"]]) {
    const output = execFileSync(process.execPath, [__dirname + "/migration-numbers.cjs", ...refs], { encoding: "utf8" });
    assert.match(output, /Migration numbering is valid/);
  }
});

test("a stack created during planning prevents automatic mutation", async () => {
  const h = harness({ finalOthers: [{ number: 43, base: { ref: "feature/migrations" },
    head: { sha: "child", repo: { full_name: "coder/coder" } } }] });
  await run(h);
  assert.equal(h.commits.length, 0);
  assert.ok(h.infos.some((message) => message.includes("Discarding stale")));
});

test("overlapping rename chains never delete an addition destination", async () => {
  const laterFixture = rules.FIXTURES + "000003_pre_ours.up.sql";
  const h = harness({ head: { ...head, ...pair(3, "ours"), [laterFixture]: head[fixture] }, additionCount: 6 });
  await run(h);
  assert.equal(h.commits.length, 1);
  assert.equal(h.commits[0].fileChanges.deletions.length, 3);
});

test("workflow checks without issue writes or usable App credentials", () => {
  const { readFileSync } = require("node:fs");
  const workflow = readFileSync(__dirname + "/../workflows/migration-numbers.yaml", "utf8");
  assert.doesNotMatch(workflow, /^      issues:/m);
  assert.match(workflow, /id: app-token\n        continue-on-error: true/);
  assert.match(workflow, /ref: main\n          persist-credentials: false/);
  assert.match(workflow, /permission-contents: write/);
  assert.doesNotMatch(workflow, /permission-(?:workflows|issues|pull-requests): write/);
  assert.match(workflow, /writer: token \? getOctokit\(token\) : undefined/);
});


test("existing migration and fixture destinations prohibit repair even when the projected tree validates", () => {
  const inherited = { ...base, ...pair(2, "taken"), ...pair(3, "ours") };
  const result = rules.plan(inherited, main, { ...inherited, ...added });
  assert.match(result.errors.join("\n"), /Rename destination exists/);
  assert.equal(result.repairable, false);
  assert.ok(result.renames.some(({ to }) => to === rules.MIGRATIONS + "000003_ours.up.sql"));

  const inheritedFixture = rules.FIXTURES + "000003_pre_ours.up.sql";
  const fixtureBase = { ...inherited, [inheritedFixture]: head[fixture] };
  const withFixture = rules.plan(fixtureBase, main, { ...fixtureBase, ...added, [fixture]: head[fixture] });
  assert.ok(withFixture.errors.includes("Rename destination exists: " + inheritedFixture));
  assert.equal(withFixture.repairable, false);
});

test("number overflow prohibits a partial rename plan", () => {
  const nearLimit = { ...base, ...pair(999998, "latest") };
  const result = rules.plan(base, nearLimit, { ...head, ...pair(3, "later") });
  assert.ok(result.errors.includes("Migration number exceeds six digits."));
  assert.ok(result.renames.some(({ to }) => to.includes("999999_ours")));
  assert.equal(result.repairable, false);
});

test("opened, reopened and main-refresh events leave unlabeled conflicts untouched", async () => {
  for (const options of [{ action: "opened" }, { action: "reopened" }, { event: "push" }]) {
    const h = harness({ ...options, pr: { labels: [] } });
    await run(h);
    assert.equal(h.commits.length, 0);
    assert.equal(h.statuses.at(-1).state, "failure");
    assert.match(h.statuses.at(-1).description, /Autofix is disabled/);
    assert.deepEqual(h.pr.labels, []);
  }
});

test("an unrelated unlabeled PR succeeds without opting into autofix", async () => {
  const h = harness({ head: base, action: "opened", pr: { labels: [] } });
  await run(h);
  assert.equal(h.statuses[0].state, "success");
  assert.equal(h.commits.length, 0);
  assert.deepEqual(h.pr.labels, []);
});

test("manually adding the label enables repair; removal disables further repairs", async () => {
  const h = harness({ action: "opened", pr: { labels: [] } });
  await run(h);
  assert.equal(h.commits.length, 0);
  assert.equal(h.statuses.at(-1).state, "failure");

  h.pr.labels = [{ name: rules.LABEL }];
  h.context.payload.action = "labeled";
  await run(h);
  assert.equal(h.commits.length, 1);

  h.pr.labels = [];
  h.context.payload.action = "unlabeled";
  await run(h);
  assert.equal(h.commits.length, 1);
  assert.match(h.statuses.at(-1).description, /Autofix is disabled/);
  assert.deepEqual(h.pr.labels, []);
});
