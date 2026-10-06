# docsredirectscheck

`docsredirectscheck` validates `docs/redirects.json`, the list of redirects the
documentation website applies for pages that have moved or been removed. It
runs as `make lint/docs-redirects` (part of `make lint` and `make lint-light`),
so a bad redirect is caught in the pull request that introduces it.

A missing `docs/redirects.json` passes.

## The rule file

`docs/redirects.json` is a JSON array of rules:

```json
[
  {
    "source": "/docs/about/architecture",
    "destination": "/docs/about",
    "permanent": true
  },
  {
    "source": "/docs/tutorials/ai-agents/:path*",
    "destination": "/docs/ai-coder/:path*"
  }
]
```

- `source` and `destination` are full site paths that start with `/docs/`.
  Write them without a version segment such as `/@main`; the docs website adds
  that itself.
- `permanent` is optional and defaults to `true`.
- A `source` is an exact path, a trailing `/:name*` (matches the prefix and
  everything beneath it), or a trailing `/:name(.*)` (matches everything
  beneath the prefix, not the prefix itself).
- A `destination` is an exact path, a trailing `/:name` or `/:name*` that reuses
  the source's parameter, or an `http(s)://` URL.

## What it catches

These fail the check:

- **A malformed file.** Not valid JSON, not an array, an entry that is not an
  object, a missing or empty `source` or `destination`, a non-boolean
  `permanent`, or an unknown field.
- **Unsupported syntax.** A source outside `/docs/`, a version segment, or any
  pattern beyond the forms above.
- **A destination that is not a page.** An internal destination must be a route
  in `docs/manifest.json`. A pattern destination must point into a section that
  has pages, and may only use a parameter the source defines.
- **A source that hides a live page.** Redirects apply before pages, so a rule
  whose source matches a page in the manifest makes that page unreachable.
- **Duplicates, chains, and loops.** Two rules with the same source, a rule
  whose destination is another rule's source (point it at the final page
  instead), and cycles.

## Removed-route warning

On a pull request the check also compares `docs/manifest.json` with the base
branch. If routes were removed, `docs/redirects.json` was not changed, and no
existing rule already covers the removed routes, it prints a warning. In GitHub
Actions the warning is an annotation on the manifest. It never fails the run,
because pages are sometimes removed on purpose.

The comparison is by route, not by file, so renaming `admin/foo/index.md` to
`admin/foo.md` (same URL) is not a removal.

In GitHub Actions the base is the first parent of the pull request's merge
commit, which is the tip of the branch the change merges into. Elsewhere pass
`-base <rev>`; with neither, or when the revision is unavailable (a shallow
checkout), the warning is skipped with a note.

## How it decides which pages exist

It derives routes from `docs/manifest.json` exactly as the docs website does:
strip a leading `./`, then strip the suffixes `README.md`, `index.md`, and
`.md` one after another, trim slashes, and prefix `/docs`. That includes the
website's quirk of matching the suffixes as plain text rather than per path
segment, so a file named `my-index.md` becomes the route `/docs/.../my-`. Every
manifest entry with a path counts as a page, whether or not it appears in the
sidebar. The landing page `/docs` always exists, and `/docs/about` exists when
the first page is titled "About".

## Usage

```sh
go run ./scripts/docsredirectscheck [-manifest path] [-redirects path] [-base rev]
```

Exit status is 0 when the file is valid or absent, 1 when it has problems, and 2
when the check itself could not run (for example the manifest is unreadable).

## Limitations

- Chains through a pattern destination are not followed, because where such a
  destination lands depends on the URL being redirected.
- Only the three source forms above are understood. Regular-expression groups,
  version ranges, and mid-path parameters are rejected rather than guessed at.
- The removed-route warning compares routes only; it does not know whether an
  external site links to a removed page.
