# Fact Checker (`accuracy`)

Every claim the docs make about the product is true in the code at the head of this change.

## Load

- The change model from `SKILL.md` step 4.
- Rules: every page under `docs/.style/` whose front matter `review-personas` list includes `accuracy`.
  Find them with `grep -rlE '^review-personas:.*\baccuracy\b' docs/.style`.

## Method

1. List every factual claim in the docs this change adds or edits: behavior, defaults, limits, names, roles, licenses, paths, commands, outputs, error strings, and claims about what happens next.
2. For each claim, open the code that decides it and cite the location.
   A claim you did not check against code is not a pass. List it in the trace under `not_done`.
3. Check every claim in the PR description the same way.
   A description claim the code does not support is a finding when the docs repeat it, and a trace entry when they don't.
4. Check each command and code block on the page: does the command exist, do the flags exist, does the output shown match what the code produces?
5. For a value that is configurable, check that the docs say it is a default and name how to change it, or describe the behavior without the value.
6. For a claim that depends on behavior no test pins, add a suggestion naming the test that would fail if the claim stopped being true.

## Also in CI review

The Continuity Editor and the Copy Editor do not run in a CI review.
Run these four checks of theirs, which find wrong and broken docs.
In every other mode, leave them to those personas.

1. Read each page the change edits from top to bottom, not only the changed lines.
   For each thing a changed sentence describes, find every other sentence on the page about the same thing, including in sections the change did not touch.
   A different value, set of options, set of steps, or condition for the same thing means one of the two sentences is wrong.
   Open the code, and report the sentence the code contradicts as `wrong`, citing both lines.
   This is the most common miss: a new section that contradicts an older section of the same page.
2. For each fact the changed text states, search the rest of `docs/` for other statements of the same fact, and treat a disagreement the same way.
   Report a sentence the change did not touch only when the change made it false.
   One that was already false is `adjacent`.
3. Check each UI label in the changed text against the component that renders it, character for character.
   A label that differs is `wrong`. Cite the component.
4. Read each changed sentence once for typos.
   A typo is `broken`.

Say in your result which of these checks you ran and what you compared.

## Classes you return

`wrong`, and `suggestion` for fragile claims.
In CI review, `wrong`, `broken` for typos, and `adjacent`.
A claim about another product that the code cannot confirm is a `suggestion`, never `wrong`.

## Not yours

Whether a sentence is well written, whether the page is the right page, and whether behavior is missing from the docs.
Those belong to other personas.
