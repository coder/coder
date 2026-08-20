# Sync pipeline: pure logic

`sync-docs.mjs` generates the Fumadocs content tree from the repo's `docs/`
corpus and `docs/manifest.json`. The parts of that job that are pure functions
of their inputs (a path, the manifest, a set of routes) live here in
`routes.mjs` and `transform.mjs`, with no filesystem or network access and no
top-level side effects. `sync-docs.mjs` keeps the I/O and calls these builders.

Keeping the logic here matters because this is where a slug collision or a
manifest reorder can silently drop or misorder a published page, and a bad
build ships to airgapped customers. Pure functions let `routes.test.mjs` and
`transform.test.mjs` pin that behavior without running the full sync.

## routes.mjs

Turns docs-relative markdown paths plus the manifest into output routes, an
ordering model, and the per-directory `meta.json` page lists.

- **Directory routes and index collisions.** Every directory implied by the
  corpus is a route (for `docs/a/b/c.md`, both `a` and `a/b`). A file whose own
  route equals a directory route is emitted as that directory's `index.md`
  rather than a sibling page, so a route and a directory never both try to own
  the same URL. `buildDirRoutes` derives the directory routes; `mapMdPath`
  applies the index rule.
- **Output collisions.** Two source files can map to one output path (basenames
  that slugify the same, a case-only difference, or a file whose route collides
  with a directory index). That silently overwrites one published page with
  another, so `buildFileMap` returns every collision for the caller to fail on
  instead of shipping the loss.
- **Manifest model.** `buildManifestModel` walks `manifest.json` once and
  derives four views: route to metadata, route to the manifest path that first
  named it, route to first-seen order (document order), and directory to its
  child routes in manifest order. A route listed more than once keeps its first
  metadata and order; later occurrences are ignored, matching the sync's
  historical tolerance of duplicate manifest entries.
- **Unbacked routes.** `findUnbackedManifestRoutes` returns manifest routes that
  no source file backs (a manifest path whose file was deleted or renamed), so
  the caller can fail rather than drop them silently from the sidebar.
- **meta.json ordering.** `buildDirModel` collects the files and subdirectories
  under each directory; `buildMeta` orders them. `sortKey` compares a tuple left
  to right: bucket 0 is items in the directory's manifest child order (by that
  index), bucket 1 is items ordered only by their own manifest position, and
  bucket 2 is unordered items, which fall to the end broken by name. A directory
  sorts by the earliest-ordered page beneath it (`minOrderUnder`). The tuple
  keeps listed items ahead of unlisted ones without sentinel numbers, so there
  is no corpus-size ceiling.

## transform.mjs

Pure string transforms used by `sync-docs.mjs`. The only dependency is
fumadocs-core's frontmatter helper (a pure YAML parser). Environment specifics
(the resolved image base, the file-to-route map, image copying, source-tree
links) are injected into the rewrite helpers through a `ctx` object rather than
read from disk here.

### One fence- and blockquote-aware line scanner

`fenceScan` tracks fenced-code state one line at a time. Following CommonMark, a
fence closes only on the same marker character, a run at least as long as the
opener, and no info string, so a backtick fence cannot close a tilde block and a
short run cannot close a longer one. `stepFence` wraps it with blockquote
awareness (a fence nested in a blockquote, which `fenceScan` alone does not
see). `mapLines` is the single walker every line-level transform routes through,
so link rewriting, brace escaping, and autolink/void normalization all skip
fenced content, including blockquoted fences. `mapOutsideInlineCode` is the
matching primitive for inline code spans, so a `.md` link or `{token}` inside
backticks is never rewritten.

### HTML comments

`stripHtmlComments` removes HTML comments outside fenced code and inline code,
including multi-line comments, preserving line count so block structure is
unchanged. It returns the 1-based line of any comment that opens but never
closes so the caller can fail the sync and name the file rather than silently
swallow the rest of the page.

### Forward-looking .mdx escapes

`normalizeAngleBrackets`, `escapeCurlyBraces`, and `selfCloseVoidElements` are
no-ops at render in the current `.md` plus rehype-raw pipeline; they are chosen
to render identically today and to unblock a future `.mdx` flip, where a bare
`<`, a literal `{`, or a non-self-closed void element is a parse error. Each
skips fenced blocks and inline code spans.

### Frontmatter and title

`parseFrontmatter` parses a leading `---` block with the YAML helper (so quoting
and escapes decode properly), and only treats a block whose body is a mapping as
frontmatter, so a `---` thematic break is left as content. This matters because
`make gen` prepends a `# Code generated ... DO NOT EDIT.` YAML comment to the
API and CLI reference docs; skipping the block keeps that comment from being
read as the page H1. `extractTitle` resolves the title by precedence
(frontmatter `title`, then the first body H1 outside fences, then the manifest
title, then a title-cased route segment) and reports the lines to strip from the
body.

### Link rewriting

`rewriteTarget` rewrites one link or image target relative to its source path. A
target that escapes `docs/` (a source-tree link like `../../coderd`) can never
resolve to a synced page or bundled image, so non-image targets are pointed at
GitHub via `ctx.sourceLink` rather than left to 404; escaping images are left
alone (they would need bundling). `.md` targets resolve through `ctx.resolveMd`,
and local images through `ctx.copyImage` or a remote base.
