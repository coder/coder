# docshtmlcheck

`docshtmlcheck` fails CI when Markdown under `docs/` contains invalid inline
HTML that the documentation site's renderer silently drops or mangles. It runs
as `make lint/docs-html` (part of `make lint`).

## What it catches

- **Swallowed angle-bracket placeholders.** An unwrapped placeholder such as
  `<region>` or `<server>__` is parsed as an unknown HTML tag and stripped from
  the rendered page, so readers see broken text. Wrap placeholders in backticks
  so they render as inline code (see
  [`docs/about/contributing/documentation.md`](../../docs/about/contributing/documentation.md#placeholders-in-angle-brackets)).
  This also covers CLI `--help` strings and Swagger annotations, whose text is
  generated into `docs/reference/**`.
- **Void-element end tags** such as `</br>`. Void elements like `<br>`, `<img>`,
  and `<hr>` have no end tag.
- **Unregistered or incorrectly capitalized component tags** such as `<Image>`.
  Tag names are validated case-insensitively against the standard HTML5 element
  set (plus the `<children>` renderer component), so `<Image>` is reported
  because `image` is not a real element, not because it is capitalized.
- **Unclosed container tags**, for example a `<div class="tabs">` that is never
  closed and leaks its wrapper over the rest of the page.

## How it works

Each file is parsed with [goldmark](https://github.com/yuin/goldmark) and only
raw-HTML nodes are inspected, so angle brackets inside fenced code blocks,
inline code spans, HTML comments, and `<https://…>` / `<user@host>` autolinks
are ignored. Each raw-HTML node is tokenized as a whole with
`golang.org/x/net/html`, so a tag whose attributes wrap across lines is not
torn in half. Any tag outside the standard HTML5 element set (plus the
intentional `<children>` renderer component, which is still balance-checked) is
reported. Inline SVG and MathML are intentionally **not** in the allowed set (no
docs page uses them); add the element to `allowedElements` in `main.go` if that
changes. A finding on a generated page under `docs/reference/**` also prints a
note pointing at the generator source, since edits to the generated file do not
persist.

## Usage

```console
$ go run ./scripts/docshtmlcheck        # scans docs/
$ go run ./scripts/docshtmlcheck path/to/file.md path/to/dir
```

## Allowlist

`allowedUnknownTags` in `main.go` is a deliberately narrow, per-file escape
hatch for placeholders whose source is outside this repository (so they cannot
be fixed by a source edit here). It currently holds a temporary entry for
`docs/reference/cli/agent-firewall.md` (`<host>`/`<glob>`, generated from the
external `github.com/coder/boundary` CLI help).

The escape hatch is **self-clearing**: if an allowlisted tag no longer appears
in its file (for example once the upstream fix and dependency bump land and the
generated page no longer emits the bare placeholders), the linter reports a
`stale-allowlist-entry` and fails until the dead entry is removed, so a
suppression can never silently mask a later regression of the same tag.
