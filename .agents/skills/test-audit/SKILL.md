---
name: test-audit
description: "Use when writing, changing, reviewing, or auditing tests. Apply a value gate to new tests and identify low-value, implementation-coupled, or duplicate tests and the test-only production seams they demand."
---

# Test Audit

Three modes, one value bar. Authoring mode gates new or changed tests at write
time. Audit mode runs focused sweeps of tests that re-assert source, duplicate
stronger proof, couple behavior to implementation, or keep test-only production
seams alive. Campaign mode audits one whole subsystem's test surface; before
starting one, read [CAMPAIGN.md](CAMPAIGN.md).

Match the mode to the request. Writing or reviewing a test does not start a
repository-wide cleanup. Continue broad audits as separate coherent follow-up
changes; optimize for confidence, not deletion count.

The production owner is the component responsible for the behavior. Its owning
boundary is where that behavior can be exercised and observed independently of
its internal implementation; choose the narrowest boundary that proves the
contract, not automatically an end-to-end test.

## Authoring gate

Before adding or changing a test, answer four questions; a missing answer means
the test is not ready:

1. What observable behavior, invariant, or independent contract does it protect?
2. What credible regression makes it fail?
3. Why does existing coverage not already catch that failure? Each contract has
   one primary test owner at the strongest appropriate boundary; another layer
   needs its own distinct risk, such as a transport or lifecycle failure the
   owner cannot reach. Prefer extending a table-driven case or shared fixture
   over a near-duplicate test; consolidate duplicated setup in the same change.
4. Does it need a production seam (export, flag, wrapper, injection hook) that no
   production caller needs? If yes, move the test to the real boundary instead.

Then check the test against every [junk pattern](#junk-patterns); a match fails
the gate unless the [retention bar](#retention-bar) names the contract it
independently guards. A test that would break under behavior-preserving
refactoring is suspect; rewrite it at the owning boundary unless it independently
enforces a retained contract.

Bug regression tests must fail on the pre-fix code for the intended reason and
pass after the owner-boundary repair. If the pre-fix failure cannot be reproduced,
report that limitation instead of claiming the regression is proven. One
regression at the owner boundary covers the bug; another layer needs a distinct
failure mode, not a replay of the same scenario.

Every new test must also show that it can fail. Break the behavior it protects
in the production code (remove the new path or revert the fix; never edit the
assertion or the fixture), run the test, record the failing output, and restore
the code. A test that still passes does not protect that behavior: rewrite it or
delete it. If the behavior cannot be broken safely, report that limitation.

## Junk patterns

Use this checklist in authoring and audit modes. A match needs investigation
against the retention bar, not automatic deletion:

- assertion-free coverage probes;
- self-comparisons and identity copiers;
- copied fixtures, inventories, manifests, or export lists;
- exact source, import, or string greps;
- private predicate or call-shape tests duplicated at real boundaries;
- duplicate invocations of the same contract;
- adapter-local replays of shared helpers;
- tests whose only purpose is preserving test-only exports, globals, or wrappers;
- dead production code whose only callers are tests;
- expected values produced by the helper or renderer under test;
- mocks that implement the asserted behavior, or one identical mock standing in
  for different APIs;
- fixtures that supply results or callback ordering the owner should produce,
  or persistence asserted against a store the path never writes;
- capability tests that restate declared flags instead of exercising the
  behavior the flag promises;
- negative controls that pass for an unrelated reason, such as a denial from a
  different guard or a rejection the production path never reaches;
- names or fixtures that promise more than the input and assertions exercise.

## Value bar

Tests justify their maintenance cost by protecting behavior, a credible
regression, or an independently meaningful contract. An existing test that must
change for behavior-preserving source reorganization is suspect, not
automatically deletable.

Read repository instructions, including root and scoped `AGENTS.md` files, first.
Before judging a candidate, read the complete test and production owner, its
entry point, callers, callees, sibling implementations, overlapping tests, CI
routing, and relevant history. When the test claims dependency-backed behavior,
inspect the dependency source or types directly.

## Discovery

Keep discovery read-only and report evidence before editing. Respect a review-only
request. When cleanup is already authorized, recording the evidence is a work
step, not an additional approval gate.

For broad scope, split discovery along the repository's actual production owner
boundaries, with a cross-cutting pattern sweep where useful. Use parallel
read-only agents when available and authorized; otherwise inspect the same
boundaries sequentially. Do not assume a particular directory layout.

Outside campaign mode, prefer a few high-confidence candidates over a large
speculative inventory. Hunt for the [junk patterns](#junk-patterns).

## Retention bar

Keep a test when it independently enforces a public API, SDK, protocol, config,
migration, storage, security, platform, default, exact-output, generated
cross-language, package, release, or architecture contract. Also keep:

- call ordering when order is observable behavior;
- regressions with a credible failure mode;
- source inspection when it is the cheapest independent guard: it fails when
  the contract changes (the user-facing key, byte, or path) and survives an
  identifier-only refactor;
- a retained test that fails on the baseline: treat it as a possible product
  bug, reproduce it, and repair the owner within the authorized scope or report
  the defect separately rather than deleting the test.

Static or slow is not a deletion reason. A test that resembles implementation
may still be the independent contract; prove otherwise before removing it.

## Candidate evidence

Record every field below before editing. A missing field means the candidate is
not ready for deletion:

- exact test name and location;
- what failure it can actually detect;
- non-test callers of the covered production or support seam, including public
  consumers when the seam is exported;
- stronger remaining owner-boundary proof, or why no proof is needed;
- relevant history and the reason the test or seam exists; note unavailable
  history and retain the candidate if its purpose remains uncertain;
- production or test-support deletion unlocked, or explicitly none;
- risk and the focused validation command.

## Edit shape

Choose one coherent owner-boundary batch. Delete obsolete test-only exports,
globals, wrappers, and dead production paths instead of preserving aliases.
Establish that they have no production consumers before removing them. Move
retained regressions to their canonical owners. Consolidate repeated package or
dependency assertions into one generic contract.

Do not add replacement tests that restate
the same implementation, and do not convert uncertain candidates into cleanup
to increase deletion counts.

## Validation

Discover the repository's test commands, CI routing, formatting, and review gates
from its instructions and configuration. Use its supported runner and environment,
including documented remote or resource-heavy validation procedures. Keep source
and tests stable during a validation run; use a separate checkout for concurrent
edits or negative controls.

1. Record the baseline result for affected tests, then run the smallest owner
   and sibling suites after editing.
2. For removed source greps or plan assertions, run the executable script or
   dry-run that owns the real contract.
3. Run targeted formatting and, in Git repositories, `git diff --check`.
4. Run the actual changed-file gates required by repository policy, including
   broader checks when affected boundaries require them. Verify moved tests
   are still discovered and routed by CI.
5. Inspect `git diff --numstat`; report production/tooling separately from tests
   and test support. Treat line counts as accounting, not a success target.
6. After final audit edits, perform a preservation review of removed contracts
   against remaining proof and complete the repository's required review process.

Report checks that were skipped, blocked, or already failing; do not describe
unrun validation as passing.

## Landing and continuation

Follow the repository's commit, branch, PR, review, and merge workflow within the
user's authorization. This skill does not itself authorize publishing or merging.
Keep each change coherent and independently reviewable. After landing, refresh
from the current target branch and rerun read-only discovery before the next
high-confidence batch.

## Handoff

For audits, report:

- root cause and removed low-value categories;
- production owner simplifications;
- retained false positives and why they remain valuable;
- each new test with the break you made and its failing output;
- focused and full proof actually run, with limitations and baseline failures;
- production versus test LOC;
- PR and merge state, if applicable;
- named follow-ups.

For authoring or a focused review, keep the report proportional to the tests in
scope: protected contracts, the failing run of each new test, gaps or changes,
and validation evidence.

Adapted from OpenClaw's test-audit skill. See [NOTICE.md](NOTICE.md) for the pinned
source revision and MIT license notice.
