# Reviewer prompt

Fill the four placeholders and send the result as the whole prompt to one
read-only sub-agent per role. Spawn all eight in parallel.

Placeholders:

- {ROLE_FILE}: path to the role file, for example
  `.claude/skills/frontend-deep-review/roles/comments-and-docs.md`
- {TARGET}: how to get the diff (a commit range such as `$BASE..HEAD`,
  `origin/main...HEAD`, or `gh pr diff <n>`), plus the checkout path
  when the sub-agent shares this checkout
- {FILES}: the changed file list under site/ (one path per line)
- {PURPOSE}: the PR title and description, or the user's one-paragraph summary

---

You are one reviewer in a parallel frontend review of a pull request in this
repository. You own exactly the checkpoints in {ROLE_FILE}. Other reviewers own
everything else; if you notice something outside your checkpoints, put it in a
short "Outside my role" list at the end rather than expanding your scope.

Review target: {TARGET}

Purpose of the change:
{PURPOSE}

Changed files under site/:
{FILES}

Do this, in order:

1. Read {ROLE_FILE} completely. Every checkpoint in it applies to every
   frontend diff.
2. Read the FE rule summary in `site/AGENTS.md` of the checkout under review
   (and `.claude/docs/FRONTEND_PATTERNS.md` for examples) once, so you know
   the FE rule each checkpoint expands. Where they disagree with the role
   file, the role file wins.
3. Get the diff for the target, restricted to site/. Follow the role file's
   "Reading order": open whole files, sibling folders, existing components,
   or stories when the diff alone cannot answer a checkpoint. Read-only: do
   not edit, format, or create files, and do not run the app.
4. Walk every hunk against every checkpoint. For each hit, record the
   checkpoint ID, the severity (the role file's default unless the
   consequence is user-visible, which raises it, or the code is pre-existing
   and merely touched, which lowers it), the exact file:line in the new
   file, what is wrong in one sentence, and the fix in one sentence.
5. Skip findings you cannot pin to a line in the diff. Skip pre-existing code
   unless the PR is a refactor of exactly that code. Skip anything a linter
   or formatter already reports.

Your final message is the deliverable. Use exactly this layout:

## <PREFIX> findings

<ID> <severity> <file:line>
<one sentence: what is wrong>
Fix: <one sentence>

(repeat per finding; a checkpoint may appear several times)

## Verdict

<ID> PASS | FAIL | N/A
(one line per checkpoint in the role file, in ID order; N/A only when the
diff contains nothing the checkpoint could apply to)

## Outside my role

- <prefix that owns it> <file:line> <one sentence>
(or "none")

If nothing failed, the findings section reads "No findings." and every
verdict line is PASS or N/A. Keep the message free of preamble and method
narration; the orchestrator merges eight of these.
