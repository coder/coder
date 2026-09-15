# Path priors

What the changed paths predict about whether a change needs docs. A prior sets
the starting assumption and the burden of proof; it never replaces the evidence
discipline in [`SKILL.md`](./SKILL.md). Verify against the diff either way.

Three classes. The first is decided before this skill runs.

## Decided in CI: no documentation surface

Tests and stories, generated output, dependency and toolchain manifests, build
and lint plumbing, styling, internal environments, vendored trees. When *every*
changed file is in this class, `.github/workflows/doc-check.yaml` skips the
review and no chat starts, so you will not see these diffs.

Two consequences:

- A mixed diff still reaches you. One qualifying file is enough to start a
  review, so do not assume every file in front of you has a documentation
  surface.
- The skip is overridable. A maintainer can force a review with the `doc-check`
  label, which means you can be handed a diff the skip would otherwise have
  dropped. Review it on its merits.

The authoritative list is the `nodocs` filter in that workflow. Docs-affecting
CI is carved back out of it, because a change to how the docs build, deploy,
preview, or get reviewed can change the docs themselves.

## Strong prior: absent docs is the finding

These paths add or change a surface a user configures, types, calls, or lands
on. Missing documentation here is a gap, not a judgment call. Start from
"this needs docs" and look for the page that proves otherwise.

| Change | Why it earns docs |
|---|---|
| New or changed `cli/` command or flag | Appears in `--help`, so it is user-facing by default |
| New or changed public `codersdk/` field or endpoint | The API is a first-class user surface |
| New `coderd/apidoc/` endpoint | Same, and the path must be documented in full |
| New or changed deployment option or `CODER_*` environment variable | Configuration is product surface area |
| New `site/src/pages/` route | A new navigable page is net-new surface |
| New licensed `enterprise/` feature, or a new Premium gate | Needs the page plus both Premium markers |
| A changed setup, enablement, or migration procedure | The steps a user follows are the documentation |
| Renamed or deprecated product or feature name | The glossary and every referring page drift otherwise |

Two cautions that survive a strong prior:

- An auto-generated reference page regenerating is not documentation. A new flag
  landing in `docs/reference/cli/` still leaves a conceptual guide gap.
- A surface that is not visible by default, including anything behind an unsafe
  experiment, has not reached the bar yet.

## No prior: decide from the diff

Everything unlisted, including component-level frontend changes,
`coderd/` handler behavior, provisioner changes, and database migrations. A
migration alone has no user surface, but it often ships next to an API change,
and the mixed-diff rule above means you will see that change too.

Here the path tells you nothing and the evidence discipline decides. This is
also where the near-miss lives: a page enumerates a sibling surface rather than
the one that changed. Resolve it under the finding bar in `SKILL.md`, and prefer
naming no page over naming the nearest one.
