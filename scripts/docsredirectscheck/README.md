# docsredirectscheck

`docsredirectscheck` validates `docs/redirects.json`, the list of redirects the documentation websites apply for pages that have moved or been removed.
It runs as `make lint/docs-redirects` (part of `make lint` and `make lint-light`), so a bad or missing redirect is caught in the pull request that introduces it.

Each branch has its own `docs/redirects.json`.
The file on `main` applies to `@main`, and the file on `release/X.Y` applies to vX.Y and, while X.Y is the mainline release, to the unversioned docs.
A missing `docs/redirects.json` passes.

## The rule file

`docs/redirects.json` is a JSON array of rules:

```json
[
  {
    "source": "about/architecture",
    "destination": "/"
  },
  {
    "source": "tutorials/ai-agents/*",
    "destination": "ai-coder/*",
    "permanent": false
  }
]
```

- `source` and `destination` are routes relative to the docs root.
  A route is the URL path below `/docs` with no version, no leading or trailing slash, and no `.md`, such as `admin/templates/extending-templates`.
  Each segment matches `[A-Za-z0-9._-]+`.
- `permanent` is optional and defaults to `true`.
  `true` answers 308 and `false` answers 307.
- A `source` is an exact route, or a route ending in `/*`.
  A wildcard matches the prefix itself and every route beneath it.
- A `destination` is an exact route, `/` for the docs root, or an `https://` URL that starts with `https://github.com/coder/` or `https://coder.com/`.
  An external destination has no `:` after the scheme, no `*`, no `\`, and no `.` or `..` path segment, also when percent-encoded.
- An exact route destination may end in one `#fragment` that matches `[A-Za-z0-9._-]+`, such as `user-guides/workspace-access#web-ides`.
  The route part must be a live route.
  `/` and wildcard destinations never carry a fragment.
- A `destination` ends in `/*` only when the `source` does.
  The part the source's `*` matched replaces the destination's `*`.
  An empty match means the destination prefix itself, so `ai-coder/ai-gateway/*` with an empty match is `ai-coder/ai-gateway`.
  A wildcard source with an exact destination sends everything under the prefix to that one page.
- Rules apply in file order and the first match wins, so they must run from most specific to least.
  Put an exact rule above any wildcard that would also match it.
- Unknown fields are an error.

The old absolute form, such as `/docs/admin/old` or `/docs/x/:path*`, is rejected.
The error shows the converted rule.

## What it catches

These fail the check:

- **A malformed file.**
  Not valid JSON, not an array, an entry that is not an object, a missing or empty `source` or `destination`, a non-boolean `permanent`, or an unknown field.
- **Unsupported syntax.**
  A route outside the grammar above, the old `/docs/...` form, or an external destination outside the allowlist.
- **A destination that is not a page.**
  An internal destination must be a route in `docs/manifest.json`.
  A wildcard destination must point into a section that has pages.
- **A source that hides a live page.**
  A page always wins over a redirect, and some sites apply redirects first.
  A rule whose source matches a page in the manifest is therefore rejected.
- **Duplicates, chains, and loops.**
  Two rules with the same source, a rule whose destination is another rule's source (point it at the final page instead), and cycles.
- **A rule that can never apply.**
  An earlier rule whose source matches every route a later rule would.
  A wildcard above a rule inside its subtree is the usual cause.
  An exact rule above a wildcard on the same prefix is allowed, because it claims the bare prefix and leaves the subtree to the wildcard.
- **A removed route.**
  See the next section.

## Removed routes

A route that worked before the change must still work after it.
Each route in the inventory must be a live route in the head manifest, or be resolved by the head rules to a live route, to `/`, or to an allowed external URL.
The inventory is the union of:

- The routes in `docs/manifest.json` on the base branch.
- The routes in the live production page list for the target branch.
  The list is `https://coder.com/beta-docs/@main/llms.txt` for `main` and `https://coder.com/beta-docs/vX.Y/llms.txt` for `release/X.Y`.
  Only links under that version count.
  The version served at the unversioned `/beta-docs` path links its pages without a version segment, so when a list has no links under its own segment, the unversioned links count instead.
  When the list redirects to `/docs`, links under `/docs` count the same way.
- The `source` of every rule in `docs/redirects.json` on the base branch.
  A wildcard source contributes its prefix.
  A base file in the old absolute form is read as well.

A failure names the route, where it came from, and a rule to paste, for example `{"source": "path/to/page", "destination": "/"}`.
The destination is the one live page with the same last segment.
Otherwise it is the one page this change added in the same parent section, which catches a rename such as `admin/integrations/istio` to `admin/integrations/istio-mesh`.
Otherwise it is `/`.
Change the destination when a better page exists.
In GitHub Actions each failure is also an error annotation.

Both ends of the check can be missing, and the check says so and skips that part:

- **Base branch.**
  In GitHub Actions on a pull request the base is the first parent of the merge commit, which is the tip of the branch the change merges into.
  Elsewhere pass `-base <rev>`.
  A shallow checkout that lacks the revision skips the comparison with a note.
- **Live page list.**
  The target branch is `-branch`, then `GITHUB_BASE_REF` on a pull request, then `GITHUB_REF_NAME` on a push.
  A branch other than `main` or `release/X.Y`, or no known branch, skips the list with a note.
  A list that answers 404 skips the list with a note, because a new `release/X.Y` branch has no published docs version yet.
  When `docs/manifest.json` and `docs/redirects.json` are the same as on the base branch, the list is skipped with a note, because the change cannot affect the result.
  On a pull request, a coder.com outage only blocks changes to those two files.
  Pass `-offline` to skip the list on purpose, for example without network access.

A live page list that cannot be read for any other reason is an infrastructure failure and exits 2, not a pass.
The request has a descriptive `User-Agent`, because the site answers 403 to some default clients, and it retries a few times before it gives up.
An empty list is also a failure.

A base `docs/redirects.json` that does not exist means the base branch had no rules.
Any other failure to read it skips the base redirect sources with a note.

### Dotted segments

beta-docs currently serves a dotted segment with hyphens, so `esr-2.24-2.29-upgrade` is served as `esr-2-24-2-29-upgrade`.
Until the docs engine fixes it, a live route that equals a head route with `.` replaced by `-` counts as present.

## How it decides which pages exist

It derives routes from `docs/manifest.json` as the docs websites do: strip a leading `./`, then strip the suffixes `README.md`, `index.md`, and `.md` one after another, and trim slashes.
That includes the websites' quirk of matching the suffixes as plain text rather than per path segment, so a file named `my-index.md` becomes the route `.../my-`.
Every manifest entry with a path counts as a page, whether or not it appears in the sidebar.
The empty route is the docs root, and it always exists.

## Usage

```sh
go run ./scripts/docsredirectscheck [-manifest path] [-redirects path] [-base rev] [-branch name] [-offline]
```

Exit status is 0 when the file is valid or absent, 1 when it has problems, and 2 when the check itself could not run.
Examples are an unreadable manifest or a live page list that cannot be read.

## Limitations

- Chains through a wildcard destination are not checked, because where such a destination lands depends on the route being redirected.
  The removed-route check does follow rules, up to 3 hops, whatever the last destination is.
- Only the two source forms above are understood.
  Regular-expression groups, version ranges, and mid-path parameters are rejected rather than guessed at.
- The check does not know whether an external destination is reachable.
