# styleclaims

`styleclaims` fails CI when the Coder documentation style guide claims a prose
rule is enforced by tooling that isn't actually enabled. It runs as
`make lint/style-claims` (part of `make lint` and `make lint-light`).

## Why it exists

Each rule section in [`docs/.style/style-guide/`](../../docs/.style/style-guide/README.md)
ends with an annotation naming what enforces it. Those annotations drifted from
the configuration: 8 cited third-party `Google.*` rules that the repo-root
`.vale.ini` never loads, and 2 cited `Coder.*` rules that have no YAML under
`docs/.style/styles/Coder/`. A reader who trusts an annotation believes a rule
is checked when it isn't, which is worse than an unenforced rule the guide is
honest about.

## What it catches

- **A `Coder.*` rule cited as active with no rule file.** The rule doesn't
  exist; the citation needs `(planned)`.
- **A third-party rule cited as active.** `.vale.ini` sets
  `BasedOnStyles = Coder`, and third-party rules return one per PR after their
  corpus is clean, so any `Google.*`, `alex.*`, or `write-good.*` citation is
  planned until that happens.
- **A rule with no style guide section.** A rule file exists but no annotation
  cites it as active, so a rule landed without the section the per-rule PR
  pattern requires.
- **Checks-table drift.** A severity that disagrees with the rule file's
  `level:`, a row for a rule that doesn't exist, a missing row for a rule that
  does, or a scope cell that doesn't name the globs where `.vale.ini` enables
  the rule.
- **Coverage-table drift.** A per-section or total count that disagrees with the
  annotations.

## The annotation format it relies on

An annotation starts with one of `Enforced`, `Documentation-only`, `Adapted`,
`Vale rule`, `Periods`, or `Alt-text`, and runs to the line ending in `*`.

A citation counts as a claim only when its sentence contains `enforc` or names a
`Vale rule`. A sentence that mentions a rule without claiming it runs, such as
the note explaining why `Google.Passive` stays out of the package, is ignored.

Mark each planned citation individually. `(planned)` after the citation, or
`Planned Vale rule` before it, both work:

```md
*Enforced by `Coder.BrandNames`.*
*Enforced by `Coder.LearnMore` (planned).*
*Enforced by `Google.Gender` (planned) and `Google.GenderBias` (planned).*
*Enforced by `scripts/check_emdash.sh` (existing CI script) and `Coder.EmDash` (planned).*
*Documentation-only. No Vale rule.*
```

A trailing clause such as `(both planned)` that covers several citations at once
isn't understood; mark each one.

## Counting

Each annotation is classified once for the coverage table:

- **Tool-checked**: cites an existing Coder rule, `markdownlint`, an `MD###`
  rule, or `scripts/check_emdash.sh` as active.
- **Planned**: cites only planned checkers.
- **Documentation-only**: everything else.

## Usage

```sh
go run ./scripts/styleclaims
```

It takes no arguments and runs from the repository root. Findings print as
`file:line: message` and exit 1.
