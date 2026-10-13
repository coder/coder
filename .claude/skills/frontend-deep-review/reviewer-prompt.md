# Reviewer prompt

Fill the four placeholders and send the result as the whole prompt to one
read-only sub-agent per role. Run as many at once as the harness allows and
queue the rest.

Placeholders:

- {ROLE_FILE}: path to the role file, for example
  `.claude/skills/frontend-deep-review/roles/comments-and-docs.md`
- {TARGET}: the exact command that prints the diff, plus the checkout path
  when the sub-agent shares this checkout. In self-review, use
  `git diff <sha> -- site/` with the concrete merge-base SHA: it includes
  staged and unstaged edits, and a sub-agent's shell has no `$BASE`. For
  someone else's PR, use `gh pr diff <n>`.
- {FILES}: the changed file list under site/ (one path per line), with
  untracked new files marked `(new, untracked)` because the diff omits them
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
   frontend diff. Identifiers in its examples come from past reviews and
   may no longer exist in the tree: match the pattern, not the name.
2. Read the FE rule summary in `site/AGENTS.md` of the checkout under review
   (and `.claude/docs/FRONTEND_PATTERNS.md` for examples) once, so you know
   the FE rule each checkpoint expands. These files outrank the role file:
   where a checkpoint contradicts them, apply the repository rule and list
   the checkpoint under "Conflicts with repository rules".
3. Get the diff for the target and review only the sections for files
   under site/ (`gh pr diff` has no path filter, so skip the rest
   yourself). Read every file marked `(new, untracked)` in full. Follow
   the role file's "Reading order": open whole files, sibling folders,
   existing components, or stories when the diff alone cannot answer a
   checkpoint. Read-only: do not edit, format, or create files, and do not
   run the app.
4. Walk every hunk against every checkpoint. For each hit, record the
   checkpoint ID, the severity (the role file's default unless the
   consequence is user-visible, which raises it, or the code is pre-existing
   and merely touched, which lowers it), the exact file:line in the new
   file (or `(whole PR)` for a PR-scope checkpoint such as RO6), what is
   wrong in one sentence, and the fix in one sentence.
5. Skip findings you cannot pin to a line in the diff, except PR-scope
   findings such as RO6. Skip pre-existing code the diff does not touch,
   unless the PR is a refactor of exactly that code. Skip anything a
   linter or formatter already reports.

Your final message is the deliverable. Use exactly this layout:

## <PREFIX> findings

<ID> <severity> <file:line, or (whole PR)>
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

## Conflicts with repository rules

- <ID> <one sentence: the rule in site/AGENTS.md or FRONTEND_PATTERNS.md
  that the checkpoint contradicts>
(or "none")

If nothing failed, the findings section reads "No findings." and every
verdict line is PASS or N/A. Keep the message free of preamble and method
narration; the orchestrator merges eight of these.
