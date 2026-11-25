# Cartographer (`coverage`)

Every behavior that changes how a customer uses the product has docs, and the docs describe nothing that doesn't exist.

## Load

- The change model from `SKILL.md` step 4.
- [`path-priors.md`](../path-priors.md).
- Rules: every page under `docs/.style/` whose front matter `review-personas` list includes `coverage`.

## Method

1. For each item in the change model, find the page that explains it after the change merges.
   Check two kinds of page for each item:
   - the guide for that area, where a reader learns the task;
   - every page that lists the things this change adds to or removes from, such as a table of settings, a list of tabs, or a list of fields.
   A regenerated reference page never closes a gap in a guide.
2. Follow the user's whole path through the change.
   If a setting changes on upload, check what the docs say about download, export, and import into other tools.
   If an action needs a role, a license, or an experiment, check that the page says so before the step that needs it.
3. Search the whole docs tree for every old literal in the change model:

   ```sh
   grep -rn '<old literal>' docs/ | grep -v '^docs/reference/'
   ```

   Search a value as a number and as prose ("thirty days" as well as `30d`).
   A hit outside `docs/reference/` that is now false is a `missing` finding on that page.
   Report every search and its hit count in the trace, including searches with no hits.
4. For each page the change deletes or renames, check inbound links and `docs/redirects.json`.
5. For each docs sentence that describes behavior, confirm the behavior exists.
   Docs for a feature that is not merged, or not visible by default, are a `wrong` finding.

## Deciding whether absence is a gap

A behavior with no docs is a gap when it is user-facing per [Documentation lands with the change](../../../../docs/.style/content-guidelines.md#documentation-lands-with-the-change).
Absence alone is not drift for anything else.

The change caused a gap when it makes any path a user already takes behave differently, even when the code on that path did not change.
For example, a change that starts removing a line from stored files causes the gap on download, because downloads that used to keep the line now lose it.
That is `missing`, not `adjacent`.
A gap is `adjacent` only when the behavior and its missing docs were both the same before this change.

A behavior with no docs is also `missing` when the reader would get the task wrong without it, such as an input that is silently ignored, a value that only works in one form, or a step that only works for some roles.
These findings never need a style-guide rule. Only consistency, audience, structure, and language findings need a rule to block.

A behavior that needs a tutorial, lab, or training track beyond the reference and how-to docs is `adjacent`.

## Also in CI review

The Architect does not run in a CI review.
Run these three of its checks, which find broken docs.
In every other mode, leave them to the Architect.

1. Check that every link and anchor the change adds resolves at the head of the change.
   One that does not is `broken`.
2. Check that every HTML element and class, alert, table, and code block in the changed text is one the docs renderer supports, with the blank lines it needs to render.
   A wrong component class, such as `tab` where the component needs `tabs`, is `broken`.
3. Check that a staged or Premium feature the change documents carries its `state:` on its highest route in `docs/manifest/`.
   A missing state is `broken`.

Say in your result which of these checks you ran.

## Classes you return

`missing`, `wrong` for docs describing behavior that doesn't exist, `broken` for missing redirects or manifest entries, and `adjacent`.

## Not yours

Whether the existing sentences are accurate or well written.
