# Test-audit campaign

Campaign mode audits one subsystem's whole test surface, such as a library,
service, application feature, or plugin. The value bar, retention bar, candidate
evidence, and validation in [SKILL.md](SKILL.md) apply throughout. Use one coherent
change or a sequence of independently reviewable changes according to the
repository's delivery conventions. Each step ends on its completion criterion.

## 1. Baseline

Record the subsystem's test and support line counts and every test file's result
at a pinned baseline revision, including the runtime and commands used. Keep
baseline failures separate; they may reveal product bugs rather than stale tests.
Record blocked or unavailable tests explicitly and carry those gaps into the
handoff. Missing execution evidence is not evidence for deletion.

Done when every in-scope test file has a recorded result or explicit blocker.

## 2. Lanes and inventory

Split the surface into **lanes** along production owner boundaries, not file
prefixes. Include the subsystem's cases at shared boundaries and its integration,
end-to-end, and live/QA scenarios where they exist.

Done when every test file and scenario the subsystem owns belongs to exactly one
lane, with shared ownership made explicit.

## 3. Read-only ledger per lane

Read every assigned test in full, including parameter tables, along with its
production owners, entry points, callers, history, and CI routing. Use separate
read-only agents per lane when available and authorized; otherwise work
sequentially. Each test declaration goes into a written **ledger** with one mark.
A parameterized test is one declaration unless its rows need different marks;
then mark each row.

- `R`: retain, naming the contract and the bug it catches; a retained test that
  only moves to a better-named file stays `R` with the move noted;
- `F`: retain the contract but repair the assertion, such as a vacuous negative
  that passes when only one of several items is missing;
- `C`: consolidate, naming the owner that absorbs the assertion first: a sibling
  table case, a stronger boundary suite, or the shared owner in another package;
- `D`: delete, naming the proof that remains, or why no contract exists.

Judge a test by its assertions, not its name. Record the candidate evidence from
[SKILL.md](SKILL.md#candidate-evidence) for each proposed deletion or consolidation.

Done when every declaration in the lane has a mark and supporting evidence.

## 4. Layer plan per lane

Treat the per-test ledger as input, not as the edit list. A second read-only pass
looks for redundant layers: several suites may replay one shared helper through
mocked collaborators while a stronger suite already exercises the real boundary.
Name the **keeper** suite for each contract. Prefer exercising the real owner
with controlled external dependencies over a mock that implements the behavior
being asserted. Correct ledger errors found in this pass.

Done when each lane plan names its retired files, its keeper per contract, the
assertions to carry into keepers, and the test-only production seams unlocked.

## 5. Cutover

Edit lane by lane. Serialize changes to shared harnesses and support files
through one owner. Establish keeper coverage before removing redundant tests.
With each lane, remove obsolete test-only production seams it unlocks, after
checking for production consumers. Register moved suites in CI routing and test
inventories. Update existing size baselines if the repository uses them; do not
weaken coverage or review gates to make the campaign pass. Record durable
test-ownership lessons in existing subsystem guidance when useful and in scope.

Done when every lane plan is applied and each lane's keepers pass, with unresolved
baseline failures and blocked validation explicitly accounted for.

## 6. Preservation review

Before claiming completion, compare deleted coverage against the keepers by
boundary group. Use independent reviewers when available and authorized;
otherwise perform a separate review pass and disclose that it was not independent.
Look for contracts that lost their only proof and new assertions that cannot
fail, such as a rejection case the production code never reaches.

For each restored contract, make one deliberate **mutation** of the production
owner in an isolated checkout and confirm the keeper fails for the intended
reason. Restore the source exactly afterward and rerun the passing keeper. A
compile error, unrelated failure, or unreachable path does not prove detection.

Done when every reported gap is restored or rejected with source evidence, and
every restored contract has a caught mutation. Record any blocked proof as an
outstanding gap rather than claiming preservation is verified.

## 7. Product defects

Investigate a baseline failure that survives into a keeper as a possible product
defect. Within the authorized scope, fix it at its owner as a separate commit
and prove it through the real behavior, with a **control** run that removes only
the fix and demonstrates the old failure. Use the same harness for both runs.
Report unrelated defects as follow-ups instead of expanding the campaign.

Done when each repaired defect has a failing control and a passing candidate on
the same harness, with deferred defects explicitly recorded.

## 8. Reconcile and hand off

Refresh from the current target branch using the repository's merge or rebase
policy. When upstream modified a file the campaign deleted, inspect the new
coverage and preserve every newly protected contract in an appropriate keeper;
revisit the deletion if that is no longer possible. Rerun affected validation on
the reconciled head, including the whole subsystem suite and applicable live
proof before claiming full campaign validation.

Check that review tools covered the full diff; large file lists may be truncated.
Record maintainer decisions and validation limitations in the review evidence.

Hand off with the [SKILL.md](SKILL.md#handoff) report, plus:

- baseline and final test/support line counts, with production counted separately;
- lanes, retired layers, and keepers;
- preservation gaps found and their mutations;
- product defects with control and candidate proof;
- unresolved baseline failures, blocked checks, and deferred follow-ups.
