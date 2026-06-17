# Coder custom Vale rules

Custom Vale rules specific to Coder live here. Each rule is a YAML file
that Vale loads through the `BasedOnStyles = Coder` setting in the
repo-root `.vale.ini`.

This directory is intentionally empty for now. The rule-specific tickets
in the
[Docs style guide](https://linear.app/codercom/project/docs-style-guide-7828445b9afc)
Linear project add rules incrementally:

| Ticket                                                 | Rule                               |
|--------------------------------------------------------|------------------------------------|
| [DOCS-33](https://linear.app/codercom/issue/DOCS-33)   | Dev Container terminology          |
| [DOCS-34](https://linear.app/codercom/issue/DOCS-34)   | HashiCorp casing                   |
| [DOCS-35](https://linear.app/codercom/issue/DOCS-35)   | Limit "we"                         |
| [DOCS-36](https://linear.app/codercom/issue/DOCS-36)   | Setup vs set up, Quickstart casing |
| [DOCS-37](https://linear.app/codercom/issue/DOCS-37)   | Next steps vs Learn more           |
| [DOCS-41](https://linear.app/codercom/issue/DOCS-41)   | Vale substitution rule scaffold    |
| [DOCS-42](https://linear.app/codercom/issue/DOCS-42)   | Weasel words                       |
| [DOCS-44](https://linear.app/codercom/issue/DOCS-44)   | Em-dash and en-dash mirror in Vale |
| [DOCS-182](https://linear.app/codercom/issue/DOCS-182) | Inclusive-language substitutions   |
| [DOCS-183](https://linear.app/codercom/issue/DOCS-183) | Product-voice rules                |
| [DOCS-191](https://linear.app/codercom/issue/DOCS-191) | Gerund-leading headings            |

## Authoring a new rule

1. Write a YAML file under this directory. Name it after the rule's
   intent, for example `InclusiveLanguage.yml` or `ProductVoice.yml`.
2. Each rule's `message:` should link to the matching section in
   `docs/.style/style-guide.md`, ideally with a deep-link anchor, so a
   contributor reading a Vale warning can jump straight to the guidance.
3. Land at `level: warning` first. Promote to `level: error` only after
   both conditions hold:
   - The rule is objectively correct (typo, brand-name casing, banned
     substitution).
   - The existing-content violation count for the rule reaches zero.
4. The
   [parity CI check](https://linear.app/codercom/issue/DOCS-179) will
   eventually verify that every rule here has a matching section in
   `style-guide.md`. Add the section in the same PR as the rule.

## Reference

- Vale docs: <https://vale.sh/docs/>
- Vale rule types: <https://vale.sh/docs/topics/styles/>
