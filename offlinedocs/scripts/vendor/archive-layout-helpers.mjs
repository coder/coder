// Layout of a single-version export as the exact URL tree the router serves
// from the archive, plus the publish index the publisher uploads beside it.
// Also the relocation every build mode ends with: the markdown twins move
// from the staging directory the routes generate into to the page URLs they
// are served at.
import { mkdir, readdir, rename, rm } from "node:fs/promises";
import path from "node:path";
import { isVersionShapedSegment, versionUrlPrefix } from "@coder/docs-manifest/version-segment";
import { contentTypeFor } from "@coder/docs-manifest/content-types";

const stripLeadingSlash = (value) => value.replace(/^\/+/, "");

// The directory the twin routes generate into. A Next route segment cannot
// match on a file extension, so no route can emit `<page>.md` beside the page
// catch-all that resolves the same path; the routes write here and
// `relocateMarkdownTwins` moves each file to its published URL.
const MARKDOWN_STAGING_DIR = "llms.mdx";

// The plain-text channel, relative to whatever version prefix it sits under:
// the two index files and the per-page markdown twins, whether they are still
// staged under `llms.mdx/` or already relocated. A twin is the only `.md`
// file an export holds, so the extension identifies a relocated one wherever
// it sits: the only corpus files copied into `public/` are images, and
// `IMAGE_EXTENSIONS` in @coder/docs-config does not admit `.md`. The tests
// pin that, because widening that list would make this predicate drop a real
// asset.
export const isPlainTextChannel = (rest) =>
  rest === "llms.txt" ||
  rest === "llms-full.txt" ||
  rest === MARKDOWN_STAGING_DIR ||
  rest.startsWith(`${MARKDOWN_STAGING_DIR}/`) ||
  rest.endsWith(".md");

/**
 * The URL a staged twin is served at: its path without the staging segment,
 * so `v2.36/llms.mdx/admin/users.md` becomes `v2.36/admin/users.md` and
 * `llms.mdx/index.md` becomes `index.md`. Undefined for anything else, a
 * twin that has already been relocated included.
 *
 * `users.md` sits beside the `users/` directory holding the page's own
 * `index.html`, and `index.md` beside `index.html`, so no twin collides with
 * the page tree it is a copy of. Only a `.md` file is a twin: anything else
 * under the staging segment is left where it is, and the on-disk step then
 * refuses to remove a staging directory that is not empty.
 *
 * @param {string} file - an export-relative path with forward slashes.
 * @returns {string | undefined}
 */
export function relocatedTwinPath(file) {
  if (!file.endsWith(".md")) return undefined;
  const segments = file.split("/");
  const at = segments.indexOf(MARKDOWN_STAGING_DIR);
  if (at === -1 || at === segments.length - 1) return undefined;
  return [...segments.slice(0, at), ...segments.slice(at + 1)].join("/");
}

/**
 * Every staged twin's rename, refusing a target the export already holds:
 * the same collision discipline `layoutPlan` has, since a twin landing on a
 * file that is not its own would publish one page's bytes at another's URL.
 *
 * @param {string[]} files - export-relative paths with forward slashes.
 * @returns {{ from: string; to: string }[]}
 */
export function markdownTwinRelocations(files) {
  const present = new Set(files);
  const moves = [];
  for (const from of files) {
    const to = relocatedTwinPath(from);
    if (to === undefined) continue;
    if (present.has(to)) throw new Error(`the markdown twin ${from} collides with ${to}`);
    present.add(to);
    moves.push({ from, to });
  }
  return moves;
}

/**
 * Apply those renames inside an export directory and remove the emptied
 * staging directories, so no build mode publishes `llms.mdx/`.
 *
 * @param {string} dir - the export root (out/).
 * @returns {Promise<number>} how many twins moved.
 */
export async function relocateMarkdownTwins(dir) {
  const entries = await readdir(dir, { recursive: true, withFileTypes: true });
  const relativeOf = (entry) =>
    path.relative(dir, path.join(entry.parentPath, entry.name)).split(path.sep).join("/");
  const files = entries.filter((entry) => entry.isFile()).map(relativeOf);
  // Every staging directory the export holds, read from the tree rather than
  // derived from the moves below. A staging directory with no twin under it
  // produces no move, so deriving the set from the moves would leave exactly
  // that directory in place to be published as `llms.mdx/` — the one outcome
  // this step exists to prevent. Deepest first, so a nested staging
  // directory is judged and removed before the one containing it.
  const staging = entries
    .filter((entry) => entry.isDirectory() && entry.name === MARKDOWN_STAGING_DIR)
    .map(relativeOf)
    .toSorted((a, b) => b.split("/").length - a.split("/").length);
  const moves = markdownTwinRelocations(files);
  for (const { from, to } of moves) {
    const target = path.join(dir, ...to.split("/"));
    await mkdir(path.dirname(target), { recursive: true });
    await rename(path.join(dir, ...from.split("/")), target);
  }
  // Every file under a staging directory should be a twin, so what is left
  // of it is its own empty subdirectories. Anything else is a build the
  // routes did not produce, and it is refused rather than deleted or
  // published under `llms.mdx/`.
  for (const relative of staging) {
    const root = path.join(dir, ...relative.split("/"));
    const leftover = (await readdir(root, { recursive: true, withFileTypes: true })).filter(
      (entry) => entry.isFile(),
    );
    if (leftover.length > 0) {
      throw new Error(
        `${relative}/ holds a file that is not a markdown twin: ${relativeOf(leftover[0])}`,
      );
    }
    await rm(root, { recursive: true, force: true });
  }
  return moves.length;
}

// The version prefix an exported path carries (`v2.36`, `@v2.36.4`), or "" for none.
const versionSegmentOf = (file) => {
  const first = file.split("/", 1)[0];
  return isVersionShapedSegment(first) ? first : "";
};

/**
 * The plain-text files a stitched export carries under the ROOT version's own
 * `v<id>` alias prefix, which nothing should serve from there.
 *
 * `createVersionedLlmsRoutes` generates that alias unconditionally, because
 * `output: "export"` refuses a dynamic route that emits no path and the
 * prefixed routes are otherwise empty in the `latest` pass. On the publisher
 * path `layoutPlan` already drops those copies; the stitch has no layout step,
 * so without this the stitched deployment grows a `v<latest>/` directory
 * holding a byte-identical duplicate of the root `llms.txt`, `llms-full.txt`,
 * and the root page's twin — beside page URLs under the same prefix that 404,
 * since the root version's pages are laid out at the root.
 *
 * @param {string[]} files - export-relative paths with forward slashes.
 * @param {string} latestId - the id of the version marked `latest`.
 * @returns {string[]} the paths to drop.
 */
export function aliasPlainTextPaths(files, latestId) {
  const prefix = `${versionUrlPrefix(latestId)}/`;
  return files.filter(
    (file) => file.startsWith(prefix) && isPlainTextChannel(file.slice(prefix.length)),
  );
}

const SITEMAP_FILE = "sitemap.xml";

const joinRelative = (dir, file) => (dir ? `${dir}/${file}` : file);

/**
 * @param {{ basePath: string; assetPrefix: string | undefined; files: string[] }} input
 *   `files` are export-relative paths with forward slashes.
 * @returns {{ moves: { from: string; to: string }[]; dropped: string[] }}
 */
export function layoutPlan({ basePath, assetPrefix, files }) {
  const baseDir = stripLeadingSlash(basePath);
  const assetDir = stripLeadingSlash(assetPrefix ?? basePath);
  const prefixed = assetDir !== baseDir;
  // Where this build's own pages sit in the export: its version segment for an ordinary
  // version, the export root ("") for the one served at the site root.
  const pageSegment = prefixed ? stripLeadingSlash(assetDir.slice(baseDir.length)) : "";
  const dropped = [];
  const moves = [];
  for (const file of files) {
    const versionSegment = versionSegmentOf(file);
    const rest = versionSegment ? file.slice(versionSegment.length + 1) : file;
    // A static export refuses a dynamic route that generates nothing, so the
    // plain-text routes emit a path even in a build that compiled no such
    // version: a non-root build writes 404-bodied files at the export root, a
    // root build writes copies of its own files under its `v<id>` alias.
    // Neither is reachable through the router, so publishing them would only
    // add archive objects that answer 200 with `Not found` or duplicate a file
    // already served. Drop them here, where both prefixes are known.
    if (isPlainTextChannel(rest) && versionSegment !== pageSegment) {
      dropped.push(file);
      continue;
    }
    // Only the root build's sitemap is the site's; a non-root build emits an empty one.
    if (file === SITEMAP_FILE && pageSegment !== "") {
      dropped.push(file);
      continue;
    }
    const isAsset =
      file === "_next" ||
      file.startsWith("_next/") ||
      file === "api/search" ||
      file.startsWith("api/search-semantic/");
    // A twin the build has not relocated yet is laid out at its published
    // URL, so `_publish.json` records the key the router reads.
    const destination = relocatedTwinPath(file) ?? file;
    moves.push({
      from: file,
      to: joinRelative(prefixed && isAsset ? assetDir : baseDir, destination),
    });
  }
  const seen = new Set();
  for (const move of moves) {
    if (seen.has(move.to)) throw new Error(`two exported files collide at ${move.to}`);
    seen.add(move.to);
  }
  return { moves, dropped };
}

/**
 * The layout step trusts that the sync it just ran produced exactly one
 * version's content. If Turborepo strips the single-version environment and
 * sync falls back to the multi-version fixture corpus, the slug manifest
 * carries more keys than the one version being published — this throws
 * instead of laying that corpus out and publishing it as the requested
 * version.
 *
 * @param {Record<string, unknown>} slugManifest - parsed slug-manifest.json
 * @param {string} versionId - the DOCS_VERSION_ID this build is publishing
 */
export function assertSingleVersionSynced(slugManifest, versionId) {
  const keys = Object.keys(slugManifest);
  if (keys.length !== 1 || keys[0] !== versionId) {
    throw new Error(
      `archive-layout: expected the sync to have produced exactly one version (${versionId}), found ${JSON.stringify(keys)}; refusing to publish the wrong corpus`,
    );
  }
}

/** `_publish.json`: what a prefix contains, so a read-back can prove it. */
export function buildPublishIndex({ engine, entries }) {
  if (!/^[0-9a-f]{40}$/.test(engine)) throw new Error(`engine must be a 40-hex commit: ${engine}`);
  return {
    version: 1,
    engine,
    // `contentType` is advisory and nothing consumes it. The upload recomputes
    // the type from each object's own key with `contentTypeFor`, the function
    // just above, precisely because this index is written by the build job
    // that compiles untrusted coder/coder content. It stays in the index as a
    // description of what was laid out; treating it as authoritative anywhere
    // would hand that job the response header again.
    files: entries.map(({ path: objectPath, sha256, bytes }) => ({
      path: objectPath,
      sha256,
      bytes,
      contentType: contentTypeFor(objectPath),
    })),
  };
}
