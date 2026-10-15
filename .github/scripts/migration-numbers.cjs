"use strict";

const MIGRATIONS = "coderd/database/migrations/";
const FIXTURES = MIGRATIONS + "testdata/fixtures/";
const LABEL = "migration-autofix";
const CONTEXT = "migration-numbering";

function migration(path) {
  const match = path.match(/^coderd\/database\/migrations\/(\d{6})_([^/]+)\.(up|down)\.sql$/);
  return match && { path, version: Number(match[1]), stem: match[2], direction: match[3] };
}

function fixture(path) {
  const match = path.match(/^coderd\/database\/migrations\/testdata\/fixtures\/(\d{6})_([^/]+)\.up\.sql$/);
  return match && { path, version: Number(match[1]), stem: match[2] };
}

function relevant(path) {
  return (path.startsWith(MIGRATIONS) && !path.slice(MIGRATIONS.length).includes("/") && path.endsWith(".sql"))
    || (path.startsWith(FIXTURES) && path.endsWith(".sql"));
}

/** Validate pairs and fixture versions without executing SQL. */
function validate(tree) {
  const errors = [];
  const versions = new Map();
  const fixtureVersions = new Set();
  for (const [path, entry] of Object.entries(tree)) {
    if (!relevant(path)) continue;
    if (entry.type !== "blob" || entry.mode !== "100644") {
      errors.push("Migration files must be regular, non-executable files: " + path);
    }
    if (path.startsWith(FIXTURES)) {
      const file = fixture(path);
      if (!file) errors.push("Invalid fixture filename: " + path);
      else {
        if (fixtureVersions.has(file.version)) errors.push("Duplicate fixture number: " + file.version);
        fixtureVersions.add(file.version);
      }
      continue;
    }
    const file = migration(path);
    if (!file) {
      errors.push("Invalid migration filename: " + path);
      continue;
    }
    const pair = versions.get(file.version) || {};
    if (pair[file.direction]) errors.push("Duplicate migration number: " + file.version);
    pair[file.direction] = file;
    versions.set(file.version, pair);
  }
  for (const [version, pair] of versions) {
    if (!pair.up || !pair.down || pair.up.stem !== pair.down.stem) {
      errors.push("Missing or mismatched up/down pair: " + version);
    }
  }
  for (const path of Object.keys(tree)) {
    const file = fixture(path);
    if (file && !versions.has(file.version)) errors.push("Fixture has no matching migration number: " + path);
  }
  return { errors, versions };
}

/**
 * Plan only PR-added migrations against current main. The merge-base
 * distinguishes the author's files from migrations that landed on main after
 * the branch began.
 */
function plan(base, main, head) {
  const changes = Object.keys({ ...base, ...head }).filter((path) =>
    base[path]?.sha !== head[path]?.sha || base[path]?.mode !== head[path]?.mode);
  if (!changes.some(relevant)) return { errors: [], renames: [], changes, affected: false };
  const errors = [...validate(main).errors, ...validate(head).errors];
  const unsafe = changes.filter((path) => relevant(path) && base[path]);
  // Historical edits can be legitimate; only automatic repair refuses them.
  const added = changes.filter((path) => !base[path] && migration(path)).map(migration);
  const pairs = [...new Set(added.map((file) => file.version))].sort((a, b) => a - b);
  const latest = Math.max(0, ...validate(main).versions.keys());
  const renames = [];
  const safetyErrors = [];
  const refuseRepair = (error) => {
    errors.push(error);
    safetyErrors.push(error);
  };
  pairs.forEach((version, index) => {
    const expected = latest + index + 1;
    if (expected > 999999) {
      refuseRepair("Migration number exceeds six digits.");
      return;
    }
    if (version === expected) return;
    errors.push("Migration " + version + " must be " + expected + " after current base.");
    for (const path of changes) {
      const file = migration(path) || fixture(path);
      if (!base[path] && file?.version === version) {
        const start = path.startsWith(FIXTURES) ? FIXTURES : MIGRATIONS;
        renames.push({ from: path, to: start + String(expected).padStart(6, "0") + path.slice(start.length + 6), version });
      }
    }
  });
  for (const path of changes) {
    const file = fixture(path);
    if (!base[path] && file && !pairs.includes(file.version)) {
      // Fixtures for historical migrations need human review, not renumbering.
      unsafe.push(path);
    }
  }
  const sources = new Set(renames.map((rename) => rename.from));
  for (const { to } of renames) {
    if (main[to] || (head[to] && !sources.has(to))) {
      refuseRepair("Rename destination exists: " + to);
    }
  }
  const repaired = { ...head };
  for (const { from } of renames) delete repaired[from];
  for (const { from, to } of renames) repaired[to] = head[from];
  const newMigrations = Object.fromEntries(added.map((file) => [file.path, head[file.path]]));
  const structural = [...safetyErrors, ...validate(main).errors, ...validate(newMigrations).errors, ...validate(repaired).errors];
  if (added.some((file) => main[file.path]?.sha === head[file.path]?.sha)) unsafe.push("New migration already published on main.");
  return { errors, renames, changes, affected: true, unsafe,
    repairable: structural.length === 0 && unsafe.length === 0 && renames.length > 0 };
}

/** Return a reason to avoid an automated branch edit. */
function skipReason(pr, repository, stacked = false) {
  if (pr.state !== "open" || pr.base.ref !== "main") return "PR must be open and target main.";
  if (pr.head.repo?.full_name !== repository) return "Fork PRs require a manual fix.";
  if (!pr.labels.some((label) => label.name === LABEL)) return "Autofix is disabled; add migration-autofix to enable it.";
  if (stacked || /(?:ghstack|stacked|graphite)/i.test(pr.labels.map((label) => label.name).join(" "))
    || /(?:BEGIN GRAPHITE|graphite\.dev|ghstack-source-id)/i.test(pr.body || "")) {
    return "Stacked PRs require a manual fix.";
  }
  if (pr.mergeable !== true) return "Merge conflicts or unknown mergeability require a manual fix.";
  return null;
}

/**
 * Conservative reference detection prevents silently retargeting migration tests.
 */
function referenceReason(result, texts) {
  const moved = [...new Set(result.renames.map((rename) => rename.version))];
  for (const [path, content] of Object.entries(texts)) {
    for (const version of moved) {
      const number = new RegExp("(?<![0-9])0*" + version + "(?![0-9])");
      if (number.test(path) && !result.renames.some((rename) => rename.from === path)) {
        return "Migration number in filename needs a manual update: " + path;
      }
      if (number.test(content)) return "Possible hardcoded migration reference needs a manual update: " + path;
    }
  }
  return null;
}

/** Report whether text starts with a standard generated-code header. */
function generated(text) {
  return /Code generated\b.*\bDO NOT EDIT/.test(text.split("\n", 1)[0]);
}

/**
 * Status publication depends on revisions, not computed mergeability.
 */
function sameRevision(pr, current, main, currentMain) {
  return pr.head.sha === current.head.sha && main === currentMain
    && pr.base.ref === current.base.ref && pr.state === current.state;
}

/** Discard a plan if any current eligibility input changed. */
function unchanged(pr, current, main, currentMain) {
  return pr.head.sha === current.head.sha && main === currentMain
    && pr.head.ref === current.head.ref && pr.head.repo?.full_name === current.head.repo?.full_name
    && pr.base.ref === current.base.ref && pr.state === current.state
    && JSON.stringify(pr.labels.map((label) => label.name).sort())
      === JSON.stringify(current.labels.map((label) => label.name).sort())
    && pr.mergeable === current.mergeable;
}

module.exports = { MIGRATIONS, FIXTURES, LABEL, CONTEXT, migration, fixture, relevant, validate, plan,
  skipReason, referenceReason, generated, sameRevision, unchanged };

if (require.main === module) {
  const { execFileSync } = require("node:child_process");
  const git = (...args) => execFileSync("git", args, { encoding: "utf8", maxBuffer: 16 * 1024 * 1024 }).trim();
  const tree = (sha) => Object.fromEntries(git("ls-tree", "-r", "-z", sha, "--", MIGRATIONS).split("\0").filter(Boolean).map((row) => {
    const separator = row.indexOf("\t");
    const metadata = row.slice(0, separator);
    const path = row.slice(separator + 1);
    const [mode, type, oid] = metadata.split(" ");
    return [path, { mode, type, sha: oid }];
  }));
  const [base, head] = process.argv.slice(2);
  const errors = head ? plan(tree(git("merge-base", base, head)), tree(base), tree(head)).errors
    : validate(tree(base || "HEAD")).errors;
  for (const error of errors) console.error(error);
  if (!errors.length) console.log("Migration numbering is valid (including PRs without migration changes).");
  process.exitCode = errors.length ? 1 : 0;
}
