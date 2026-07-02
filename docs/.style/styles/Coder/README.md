# Coder custom Vale rules

Custom Vale rules specific to Coder live here.
Each rule is a YAML file that Vale loads through the `BasedOnStyles = Coder` setting in the repo-root `.vale.ini`.

Active rules ship as YAML files in this directory.
See the matching sections in `docs/.style/style-guide.md` for the user-facing policy each rule enforces.
Follow-up PRs add rules incrementally.
Planned coverage:

- Dev Container terminology
- Limit `we`
- Setup vs set up, Quickstart casing
- Next steps vs Learn more
- Vale substitution rule scaffold
- Weasel words
- Em-dash and en-dash mirror in Vale
- Inclusive-language substitutions
- Product-voice rules

## Authoring a new rule

1. Write a YAML file under this directory. Name it after the rule's
   intent, for example `InclusiveLanguage.yml` or `ProductVoice.yml`.
2. Each rule's `message:` should link to the matching section in the appropriate subpage of `docs/.style/style-guide/`, ideally with a deep-link anchor, so a contributor reading a Vale warning can jump straight to the guidance.
3. Land at `level: warning` first. Promote to `level: error` only after
   both conditions hold:
   - The rule is objectively correct (typo, brand-name casing, banned
     substitution).
   - The existing-content violation count for the rule reaches zero.
4. A follow-up PR adds a parity CI check that verifies every rule here has a matching section in `style-guide.md`.
   Add the section in the same PR as the rule.

## Reference

- Vale docs: <https://vale.sh/docs/>
- Vale rule types: <https://vale.sh/docs/topics/styles/>
