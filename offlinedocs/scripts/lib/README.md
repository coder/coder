# Sync pipeline: pure logic

`routes.mjs` and `transform.mjs` are the pure, side-effect-free parts of the
docs sync - the part that turns the `docs/` corpus and `manifest.json` into a
Fumadocs content tree. Keeping them here, free of filesystem and network access,
lets `routes.test.mjs` and `transform.test.mjs` pin behavior without running the
full sync. That matters because a slug collision or a manifest reorder can
silently drop or misorder a published page.

## routes.mjs

Maps docs-relative markdown paths plus the manifest into output routes, an
ordering model, and per-directory `meta.json` page lists.

- Every directory implied by the corpus is a route; a file whose route equals a
  directory becomes that directory's `index.md`, so a page and a directory never
  fight over one URL (`buildDirRoutes`, `mapMdPath`).
- Two sources that map to one output path (slug or case collisions) are returned
  for the caller to fail on rather than silently overwriting (`buildFileMap`).
- `buildManifestModel` walks the manifest once for route metadata, document
  order, and per-directory child order; a duplicate route keeps its first entry.
- Manifest routes with no backing file are returned for the caller to fail on
  (`findUnbackedManifestRoutes`).
- `buildMeta` orders each directory's pages by manifest child order, then by
  manifest position, then by name; the root's children follow the order their
  top-level section introduces them.

## transform.mjs

Pure string transforms. A single fence- and blockquote-aware line scanner
(`mapLines`) backs every prose pass, so link rewriting, comment stripping, and
the `.mdx`-forward escapes all skip fenced and inline code. It also extracts
frontmatter and the page title (ignoring the `make gen` `# Code generated` line)
and rewrites inter-doc links and images, pointing links that escape `docs/` at
GitHub. The only dependency is fumadocs-core's frontmatter parser.
