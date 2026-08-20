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

Holds the pure content transforms: slug, title, and path helpers plus the
line-level markdown rewrites (fence normalization, HTML-comment stripping,
angle-bracket and curly-brace escaping, void-element self-closing, and
link/image rewriting). These run over a single fence- and blockquote-aware line
scanner so fenced content stays opaque to the prose passes.
