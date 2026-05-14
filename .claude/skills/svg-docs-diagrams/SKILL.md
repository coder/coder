---
name: svg-docs-diagrams
description: "Author and iterate on SVG architecture diagrams embedded in Coder docs pages. Covers viewBox sizing for the docs column, readable arrow labels, the docs-page test loop with headless Chrome, and the recurring pitfalls."
---

# SVG Docs Diagrams

Use this skill when you are writing or editing an SVG diagram that ships in
`docs/images/**` and renders inside a docs page.

Coder docs render at a fixed prose column width (`max-w-[65ch]`, roughly
650 to 680 CSS pixels regardless of viewport). Every diagram is scaled to
that column. A diagram that looks great at native 1920px will be
unreadable on the page. This skill captures the conventions that produce
diagrams that read at column width and still look correct when a reader
clicks the image to view it full-size.

## When to use this skill

- Adding a new SVG to `docs/images/**` for an existing or new doc page.
- Iterating on an existing diagram (font sizes, layout, arrow routing).
- Debugging an overlap, overflow, or unreadable label complaint.
- Removing or splitting a diagram that is too dense for the column width.

Do not use this skill for PNG screenshots, icon SVGs, or third-party
brand SVGs.

## File layout

```text
docs/images/guides/<guide-name>/
  <diagram>.svg
  <flow>.svg
```

Reference each SVG from Markdown as a plain `<img>` tag so you control
the alt text and the path. The frontend renders SVGs at the column
width and lets the reader click for a full-size view, so the SVG must
look correct at both sizes.

```markdown
<img
  src="../../images/guides/<guide-name>/<diagram>.svg"
  alt="One sentence that fully describes the diagram for screen readers."
/>
```

## Authoring rules

### viewBox sizing

The docs column is ~657 CSS pixels wide. The SVG is scaled to fit. To
keep text readable on the page, target a viewBox aspect ratio where the
horizontal scale factor is no smaller than 0.5. That means a viewBox
width of about 1000 to 1180 works well. Wider than 1280 and side-card
text becomes unreadable.

For a square-ish concept diagram: `viewBox="0 0 1000 460"` is a good
default.

For a flow diagram: `viewBox="0 0 960 360"` keeps things proportionate
and matches the existing flow diagrams in
`docs/images/guides/claude-code-self-hosted-runners/`.

Always set `preserveAspectRatio="xMidYMid meet"` and **both** `width`
and `height` attributes matching the viewBox dimensions. The width
alone is not enough.

If the SVG has `width` but no `height`, browsers loading it as a
standalone document (e.g. the raw GitHub URL someone shares in chat,
or an `<img>` whose intrinsic height is computed from the file) fall
back to a default height of 100% of the viewport. The `meet`
preserveAspectRatio then letterboxes the viewBox content inside that
taller box, so the diagram appears in a sea of white space.

Good:

```svg
<svg viewBox="0 0 1000 460" width="1000" height="460"
     preserveAspectRatio="xMidYMid meet">
```

Bad:

```svg
<svg viewBox="0 0 1000 460" width="1000"
     preserveAspectRatio="xMidYMid meet">
```

The overlap checker enforces this (`svg-missing-dimensions` error).

### Text size minima

Compute the on-page font size as
`viewBoxFontSize × (657 / viewBoxWidth)`. Aim for these on-page sizes:

| Role                | On-page target | Example viewBox size at width 1000 |
|---------------------|----------------|------------------------------------|
| Layer / zone title  | 16 px or more  | 26 to 30 px                        |
| Card title          | 9 to 10 px     | 14 to 16 px                        |
| Body / mono text    | 7 to 8 px      | 11 to 14 px                        |
| Arrow label         | 7 to 8 px      | 11 to 12 px                        |
| Footnote            | 8 to 9 px      | 13 to 14 px                        |

Body text rendering below 7 px on the page is unreadable. Either shrink
the viewBox, shorten the copy, or drop the text from the diagram and
put it in the surrounding Markdown instead.

### Arrow labels

Floating gray text over a tinted zone background is hard to read,
especially at the docs column width. Always give arrow labels a white
halo so they stay legible regardless of background color:

```css
.arrow-label {
  font-size: 11.5px;
  font-weight: 600;
  fill: #111827;
  paint-order: stroke;
  stroke: #ffffff;
  stroke-width: 3px;
  stroke-linejoin: round;
}
```

`paint-order: stroke` paints the stroke before the fill, producing a
clean white outline that does not eat the glyphs. Bump
`stroke-width` until labels are clearly readable on the page; 3px in
the viewBox is usually enough.

### Card title wrapping

SVG `<text>` does not wrap. A two-word card title like
"Developer surfaces" or "Package registries" overflows a 144 px wide
card at 15 px bold. When a title is longer than about 10 to 12
characters, split it into two `<text>` elements stacked vertically:

```xml
<text class="side-title" x="44" y="100">Developer</text>
<text class="side-title" x="44" y="120">surfaces</text>
```

Account for the extra height in the card rect.

### Visible identity and lock semantics

When a runner, workspace, or process is locked to a single user, show
that on the diagram with a pill badge. Do not bury it in the
description. Example:

```xml
<rect class="badge-locked"
  x="358" y="160" width="148" height="24" rx="6"/>
<text class="badge-label badge-locked-l" x="372" y="176">
  Locked to 1 user
</text>
```

The reader should see the lock at a glance, not have to parse a
paragraph.

### Differentiating repeated items

If the diagram shows N instances of the same concept (sessions, pool
slots, replicas) and they differ in some way, surface that difference
in the labels. Three identical-looking session cards with identical
`cwd: /workspace` lines hide the per-session subtree. Use a unique
suffix per instance:

```xml
<text class="session-mono" x="264" y="324">_sessions/cse_01ST</text>
<text class="session-mono" x="438" y="324">_sessions/cse_a4Bf</text>
<text class="session-mono" x="612" y="324">_sessions/cse_9pXk</text>
```

### Always include accessible metadata

Every SVG must have `<title>` and `<desc>` elements referenced by
`aria-labelledby` on the root `<svg>`. The `<desc>` should be one or
two sentences that fully describe what the diagram shows; this is what
screen readers announce and what search indexes pick up.

```xml
<svg ... role="img" aria-labelledby="diag-title diag-desc">
  <title id="diag-title">Short title</title>
  <desc id="diag-desc">Longer prose description.</desc>
  ...
</svg>
```

## Test loop

Treat diagrams like code: change, render, eyeball, repeat. The docs
dev server is the source of truth, not native SVG rendering.

Two layers, in order:

1. **Programmatic overlap check** with `check-svg-overlaps.js`. Runs
   in under a second, catches the recurring failure modes (text
   overflow past a card, two cards intersecting, an arrow label on
   top of a card title, two texts colliding). Run this after every
   edit; the eyeball pass is a backup, not the primary check.
2. **Eyeball pass** at the actual docs page width, plus the
   click-through full-size view.

### 1. Run the programmatic checker after every edit

```bash
node .claude/skills/svg-docs-diagrams/references/check-svg-overlaps.js \
  docs/images/guides/<guide>/<diagram>.svg
```

Exit code 0 = no overlaps or overflows. Non-zero = errors. The script
reports each finding with element coordinates and the rule it
violated, so you can fix it without screenshotting.

Loop it while iterating:

```bash
while inotifywait -e modify docs/images/guides/<guide>/<diagram>.svg; do
  node .claude/skills/svg-docs-diagrams/references/check-svg-overlaps.js \
    docs/images/guides/<guide>/<diagram>.svg
done
```

The checker uses Chrome's real text metrics via `getBBox()`, so its
verdict matches what readers see. If your diagram uses unusual class
names for nested containers, pass `--allow-nesting child>parent` to
teach the checker that the nesting is intentional.

What it catches:

- **text-overflow**: a text element extends past the bounding box of
  its smallest enclosing container rect, or pokes outside a container
  that vertically contains it.
- **rect-collision**: two container rects partially overlap (allowed
  nesting is configurable).
- **rect-stacking**: two rects of the same class fully overlap, which
  usually means a stale duplicate.
- **text-text-collision**: two text elements' bboxes intersect.
- **arrow-label-on-card**: an arrow label sits on top of a card or
  badge it doesn't logically belong to.

What it does NOT catch (use the eyeball pass for):

- Visual hierarchy and "feels crowded".
- Color contrast and readability against tinted backgrounds.
- Whether the diagram is actually accurate to the system.
- Whether the click-through full-size view looks right.

### 2. Render the diagram inside the actual docs page

Start the docs dev server (see the project's existing dev instructions;
in this repo it's coder.com running on port 4001 with `DOCS_ROOT`
pointed at the coder/coder checkout).

Then render the docs page with headless Chrome and screenshot it. See
`references/render-diagram.sh` in this skill directory for a
copy-pastable script.

```bash
bash .claude/skills/svg-docs-diagrams/references/render-diagram.sh \
  http://localhost:4001/docs/ai-coder/claude-code-self-hosted-runners \
  /tmp/svg-preview/page.png
```

### 3. Click into the full-size view

The docs frontend lets the reader click the inline image to view it at
its natural size. Verify both:

- The inline rendering at column width: text is readable, no overlap.
- The clicked-open view: text is sharp, no overflow past card borders.

If only the inline version has problems, the viewBox is too wide. If
only the full-size has problems, individual elements are mispositioned.

### 4. Use a `computer_use` subagent for the click-through

Headless Chrome can capture the inline rendering, but verifying the
clicked-open full-size view and the overall feel is best done with a
`computer_use` subagent. Spawn one with a clear list of checks:

```text
For the diagram on <url>:
1. Click through to the full-size view.
2. For each of these labels, report OK or OVERFLOWS / OVERLAPS:
   - "<label 1>"
   - "<label 2>"
   ...
3. Are arrow labels readable against the zone backgrounds?
4. Save the full-size screenshot to /tmp/svg-preview/<name>.png.
Read-only. Do not modify any files.
```

Do not trust the subagent's "looks good" if it reuses an old
screenshot path. Verify the screenshot's `md5sum` changes when you
edit the SVG.

### 5. Cache busting

Next.js dev servers cache static assets. If a fresh screenshot shows
your old SVG:

- Confirm the file on disk has the new content (`md5sum` it).
- Append a cache-buster to the URL: `<svg-url>?t=$(date +%s%N)`.
- Or restart the dev server.

If `curl <svg-url> | md5sum` matches the on-disk file, the dev server
is serving the new SVG. Any stale-looking render is on the client
side; force-reload or use incognito.

## Pitfalls

### "Looks good" from a subagent without proof

Computer-use subagents sometimes claim they re-screenshotted when they
actually reused a stale file. Always verify with:

```bash
md5sum /tmp/svg-preview/<name>.png
```

If the hash matches a previous version after you edited the SVG,
re-run the screenshot yourself.

### Pixel-counting in headless Chrome is misleading

Headless Chrome can render an SVG with a sub-natural width when the
window is wider than the SVG width. Trust the docs page render, not
raw `chrome --headless ... file://.../foo.svg` output. Always test
inside the docs page at viewport 1280.

### Estimating text width

There is no SVG equivalent of `text-overflow: ellipsis`. A 15 px bold
title can fit roughly:

- 10 to 11 characters in a 144 px wide card.
- 14 to 15 characters in a 200 px wide card.
- 18 to 20 characters in a 260 px wide card.

If a title is longer than that, wrap to two lines or shrink the font.

### Stroked text via `paint-order` breaks if you forget `stroke-linejoin`

Without `stroke-linejoin: round`, the white halo around arrow labels
has sharp corners that look like glitches at small sizes. Always
include it.

### Don't `emdash` or `endash` in `<desc>` or `<text>`

`make lint/emdash` runs across the entire repo, SVGs included. Use a
period, comma, or semicolon. Use plain ASCII hyphens.

### Don't commit preview PNGs

Put preview artifacts under `/tmp/svg-preview/`. Never commit them.
Add to `.gitignore` if you accidentally `git add -A` them in.

## Reference checklist

Before you commit an SVG change:

- [ ] `node check-svg-overlaps.js` reports 0 errors.
- [ ] `viewBox` is no wider than 1280 (target 960 to 1180).
- [ ] On-page font sizes hit the targets above.
- [ ] Arrow labels use the white halo via `paint-order: stroke`.
- [ ] No card title overflows its container at the docs column width.
- [ ] Click-through to full-size still looks correct.
- [ ] `<title>` and `<desc>` exist and describe the diagram.
- [ ] `aria-labelledby` references both IDs.
- [ ] No emdash, endash, or `--` punctuation anywhere.
- [ ] Preview PNGs are not staged.
- [ ] Tested with the docs dev server, not just raw chrome on the file.
- [ ] `make lint/emdash` and `pnpm run lint-docs` pass.

## Related files

- `references/check-svg-overlaps.js`: Node script that loads the
  SVG in headless Chrome, measures every `<text>` and `<rect>` via
  `getBBox()`, and reports overflows and collisions. Run after every
  edit.
- `references/render-diagram.sh`: copy-pastable headless Chrome
  rendering script.
- `references/template.svg`: minimal SVG template with the
  conventions baked in (viewBox, arrow-label halo, title and desc,
  badge classes).
