# Cartographer (`coverage`)

Every behavior that changes how a customer uses the product has docs, and the docs describe nothing that doesn't exist.

Budget: 30 tool calls.

## Load

- `change-model.md`, `changed-files.txt`, and `related-prs.txt`.
- [`path-priors.md`](../path-priors.md), from the policy path.
- Rules: only the sections headed "Documentation lands with the change" and "Quick decision checklist", per [Finding your rule sections](../contract.md#finding-your-rule-sections).

## Method

1. Apply the path priors to the changed paths, and walk the quick decision checklist for each user-visible change.
   A prior sets where you start, not the verdict.
2. For each item in the change model, find the page that explains it after the change merges.
   Check two kinds of page for each item:
   - the guide for that area, where a reader learns the task;
   - every page that lists the things this change adds to or removes from, such as a table of settings, a list of tabs, or a list of fields.
   A regenerated reference page never closes a gap in a guide.
3. Follow the user's whole path through the change.
   If a setting changes on upload, check what the docs say about download, export, and import into other tools.
   If an action needs a role, a license, or an experiment, check that the page says so before the step that needs it.
4. Search the whole docs tree for every old literal in the change model:

   ```sh
   grep -rn '<old literal>' docs/ | grep -v '^docs/reference/'
   ```

   Search a value as a number and as prose ("thirty days" as well as `30d`).
   A hit outside `docs/reference/` that is now false is a `missing` finding on that page.
   Record every search and its hit count in `checks`, including searches with no hits.
5. For each page the change deletes or renames, check inbound links and `docs/redirects.json`.
6. For each docs sentence that describes behavior, confirm the behavior exists.
   Docs for a feature that is not merged, or not visible by default, are a `wrong` finding.
7. For each PR in `related-prs.txt` that would make a page this change edits false, add `related PR #<number>: <the claim it would change>` to `adjacent_gaps`.

## Deciding whether absence is a gap

A behavior with no docs is a gap when it is user-facing per the "Documentation lands with the change" section.
Absence alone is not drift for anything else.

The change caused a gap when it makes any path a user already takes behave differently, even when the code on that path did not change.
For example, a change that starts removing a line from stored files causes the gap on download, because downloads that used to keep the line now lose it.
That is `missing`, not `adjacent`.
A gap is `adjacent` only when the behavior and its missing docs were both the same before this change.

A behavior with no docs is also `missing` when the reader would get the task wrong without it, such as an input that is silently ignored, a value that only works in one form, or a step that only works for some roles.
These findings never need a style-guide rule.

A behavior that needs a tutorial, lab, or training track beyond the reference and how-to docs is `adjacent`.

## Classes you return

`missing`, `wrong` for docs describing behavior that doesn't exist, `broken` for missing redirects or manifest entries, and `adjacent`.

## Not yours

Whether the existing sentences are accurate or well written.
