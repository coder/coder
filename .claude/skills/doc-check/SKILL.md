---
name: doc-check
description: Reviews code changes for documentation needs and maintains the doc-check sticky comment. Use write-docs to author documentation.
---

# Documentation Check Skill

Review a change and decide whether it requires documentation. Do not author docs in this skill; use [`write-docs`](../write-docs/SKILL.md) after identifying a need.

## Canonical rules

Read the [content guidelines](../../../docs/.style/content-guidelines.md) first. They govern what belongs in `docs/`, routing, feature stages, screenshots, and structural requirements. They supersede this skill on conflicts. Use the [prose style guide](../../../docs/.style/style-guide/README.md) only when evaluating existing or proposed docs text.

## Workflow

1. Get the full diff and PR context with `gh pr view`, `gh pr diff`, or the diff method in the prompt.
2. Apply the content guide's [quick decision checklist](../../../docs/.style/content-guidelines.md#quick-decision-checklist). Verify the diff even when the PR is labeled internal, test-only, or chore.
3. Identify user-visible behavior, CLI flags, API endpoints, configuration, migration requirements, feature-stage changes, and moved or renamed pages.
4. Search `docs/` and `docs/manifest.json` for existing coverage. Do not suggest direct edits to generated files under `docs/reference/cli/`; update the CLI definitions instead.
5. Report only concrete gaps. Cite the affected code surface and the existing or proposed docs path.
6. If no documentation change is needed, do not create a comment.

## Sticky comment contract

Before commenting, find an existing PR comment containing `<!-- doc-check-sticky -->`. Update that comment instead of creating another.

- Check off `[x]` items that the PR now addresses.
- Strike through items made irrelevant by reverted or removed code.
- Add `[ ]` items for new gaps.
- Warn when an item is checked but the matching docs change cannot be verified.
- Leave the comment unchanged when findings did not meaningfully change.

Use only relevant sections:

```markdown
## Documentation Check

### Updates Needed
- [ ] `docs/path/file.md` - What needs to change
- [x] `docs/other/file.md` - This was addressed
- ~~`docs/removed.md` - No longer needed~~ *(reverted in abc123)*

### New Documentation Needed
- [ ] `docs/suggested/path.md` - What should be documented
  > ⚠️ *Checked but no corresponding documentation changes found in this PR*

---
*Automated review via [Coder Agents](https://coder.com/docs/ai-coder/agents)*
<!-- doc-check-sticky -->
```

Keep `<!-- doc-check-sticky -->` as the final line so later runs can find the comment.

## Review boundaries

Do not request published docs for content sent elsewhere by the canonical [quick decision checklist](../../../docs/.style/content-guidelines.md#quick-decision-checklist) or [routing table](../../../docs/.style/content-guidelines.md#routing-table). If a mixed diff includes user-facing behavior, comment only on that portion.

Check manifest entries, Premium signaling, generated content, redirects, and punctuation through the canonical [structural rules](../../../docs/.style/content-guidelines.md#structural-rules).
