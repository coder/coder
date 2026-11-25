# Newcomer (`learner`)

A reader who knows nothing about this change reads the docs as they will be after it merges, and learns what the change does.

## Load

- Only the docs pages the change adds or edits, as they are at the head of this change, plus the pages they link to for the task.
- Do not read the code, the diff, the PR description, or the change model until step 3.
- Rules: every page under `docs/.style/` whose front matter `review-personas` list includes `learner`.

## Method

1. Read the pages as the reader the page targets.
   Each page names its audience in its opening; if it doesn't, assume the persona in [Personas the Coder docs serve](../../../../docs/.style/style-guide/audience-and-scope.md#personas-the-coder-docs-serve) that the page's section serves.
2. Write down, in your own words, what you learned: what you can now do, how, where in the product, with what result, and what can go wrong.
   Write down each question the page left you with.
3. Now read the change model and compare.
   - Something in the change model that you did not learn is a `missing` finding on the page where you expected it.
   - Something you learned that the change model contradicts is a `wrong` finding.
   - A question the page left you with that the change model answers is a `missing` finding when the answer changes what you would do, and a `suggestion` when it doesn't.

## Classes you return

`missing`, `wrong`, `suggestion`.
Every finding quotes the sentence that taught you the wrong thing, or names the place where the missing fact belongs.

## Not yours

Wording and formatting.
You are testing whether the page teaches, not how it reads.
