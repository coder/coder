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
- **Void-element end tags** such as `</br>` (the keyboard element is `<kbd>`,
  `<br>` has no end tag, and so on).
- **Capitalized component tags** such as `<Image>` instead of `<img>`.
- **Unclosed container tags**, for example a `<div class="tabs">` that is never
  closed and leaks its wrapper over the rest of the page.

## How it works

Each file is parsed with [goldmark](https://github.com/yuin/goldmark) and only
raw-HTML nodes are inspected, so angle brackets inside fenced code blocks,
inline code spans, HTML comments, and `<https://…>` / `<user@host>` autolinks
are ignored. The extracted HTML is tokenized with `golang.org/x/net/html`; any
tag outside the standard HTML5 element set (plus the intentional `<children>`
renderer component, which is still balance-checked) is reported.

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
external `github.com/coder/boundary` CLI help), to be removed once the upstream
fix and dependency bump land.
