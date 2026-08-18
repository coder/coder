#!/usr/bin/env node
/*
 * sync step: transform the coder/coder docs corpus into a Fumadocs content
 * source (content/docs/**) plus bundled images (public/**).
 *
 * The offline bundle ships a single version (the docs tree in this same
 * checkout), rendered from committed Markdown with no network access, so the
 * result is fully self-contained:
 *   1. Inject frontmatter `title`/`description`; strip the leading H1.
 *   2. Rewrite inter-doc `.md` links to their `/route` and copy referenced
 *      images into public/, rewriting image links to root-absolute `/images/...`.
 *   3. Generate per-directory `meta.json` from docs/manifest.json for ordering.
 *   4. Emit `.md` (not `.mdx`) so raw custom tags never break the build.
 *
 * The pure string transforms live in ./lib/transform.mjs and the pure route and
 * ordering logic in ./lib/routes.mjs, so both can be tested without running the
 * sync.
 */
import {
	cpSync,
	existsSync,
	mkdirSync,
	readFileSync,
	readdirSync,
	rmSync,
	writeFileSync,
} from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
	buildDirModel,
	buildDirRoutes,
	buildFileMap,
	buildManifestModel,
	buildMeta,
	findUnbackedManifestRoutes,
} from "./lib/routes.mjs";
import {
	escapeCurlyBraces,
	extractTitle,
	normalizeAngleBrackets,
	normalizeFences,
	normalizeStepHeadings,
	rewriteContent,
	selfCloseVoidElements,
	stripHtmlComments,
} from "./lib/transform.mjs";

const scriptsDir = dirname(fileURLToPath(import.meta.url));
const siteRoot = resolve(scriptsDir, "..");
// The docs corpus lives in the repo root's docs/ tree. Allow an override so the
// generator can run against an alternate checkout.
const DOCS = resolve(
	process.env.CODER_DOCS_DIR || resolve(siteRoot, "../docs"),
);
const OUT_CONTENT = resolve(siteRoot, "content/docs");
const OUT_PUBLIC = resolve(siteRoot, "public");
// fumadocs-mdx's generated, gitignored typed source: an index of OUT_CONTENT.
const DOT_SOURCE = resolve(siteRoot, ".source");

// coder/coder repository slug and ref that source-tree links (targets outside
// the docs/ corpus) are pointed at on GitHub, since they can never resolve to a
// synced page in the offline bundle.
const SOURCE_REPO = "coder/coder";
const SOURCE_REF = "main";

if (!existsSync(join(DOCS, "manifest.json"))) {
	console.error(
		`[sync-docs] no docs manifest at ${join(DOCS, "manifest.json")}.\n\n` +
			"This script renders the coder/coder docs/ corpus, so it expects a " +
			"checkout with a docs/ directory that contains manifest.json. It reads " +
			"../docs relative to offlinedocs by default; set CODER_DOCS_DIR to point " +
			"at another checkout's docs/ directory, for example:\n" +
			"  CODER_DOCS_DIR=/path/to/coder/docs pnpm sync",
	);
	process.exit(1);
}

// --- filesystem helper -----------------------------------------------------

function walk(dir, acc = []) {
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const full = join(dir, entry.name);
		if (entry.isDirectory()) {
			if (entry.name === "node_modules") continue;
			walk(full, acc);
		} else {
			acc.push(full);
		}
	}
	return acc;
}

function routeHref(route) {
	return route === "" ? "/" : `/${route}`;
}

// pass 1: collect every markdown file, then map each to its output path + route
const allMd = walk(DOCS)
	.map((f) => relative(DOCS, f).split("\\").join("/"))
	.filter((rel) => rel.endsWith(".md"))
	.filter((rel) => !rel.startsWith(".style/"));

const dirRoutes = buildDirRoutes(allMd);
const { fileMap, collisions } = buildFileMap(allMd, dirRoutes);

// pass 2: manifest (titles, descriptions, ordering)
const manifest = JSON.parse(readFileSync(join(DOCS, "manifest.json"), "utf8"));
const { manifestMeta, manifestPathByRoute, routeOrder, childOrderByDir } =
	buildManifestModel(manifest, dirRoutes);

// Restrict output to pages referenced by the manifest so the generated sidebar
// matches docs/manifest.json exactly. Files that exist under docs/ but are not
// in the manifest (orphans) are intentionally excluded; the previous
// offlinedocs renderer likewise only rendered manifest routes.
const manifestMd = allMd.filter((rel) =>
	routeOrder.has(fileMap.get(rel).route),
);

// Two distinct source files that map to the same output path silently overwrite
// each other. Only pages that are actually written (manifest routes) can lose
// content, so restrict the collision check to those and fail below, naming every
// source involved.
const manifestSet = new Set(manifestMd);
const outputCollisions = collisions
	.map(({ outRel, sources }) => ({
		outRel,
		sources: sources.filter((rel) => manifestSet.has(rel)),
	}))
	.filter(({ sources }) => sources.length > 1);

// A manifest route that no source file backs (a doc deleted or renamed without
// updating docs/manifest.json) is silently dropped from the generated sidebar.
// Unlike the other defect classes below, nothing writes or counts it, so collect
// each such route with the manifest path that names it and fail the sync the
// same way. `backedRoutes` is every route a docs file actually maps to.
const backedRoutes = new Set(allMd.map((rel) => fileMap.get(rel).route));
const missingManifestRoutes = findUnbackedManifestRoutes(
	routeOrder,
	manifestPathByRoute,
	backedRoutes,
);

// pass 3: rewrite + write content
const copiedAssets = new Set();
// Inter-doc links whose target could not be mapped to a synced page, collected
// with the source file that contains them so the sync can name each one and
// fail instead of silently shipping a dead relative link in the offline bundle.
const unmappedLinks = [];
// Image references whose target file does not exist under docs/, collected the
// same way as unmappedLinks so a broken image reference fails the build instead
// of shipping a broken <img> in the offline bundle.
const unresolvedImages = [];
// HTML comments that open with `<!--` but never close, collected with the file
// and the 1-based line where they began. An unterminated comment would otherwise
// silently swallow the rest of the page, so it fails the sync the same way.
const unclosedComments = [];

function copyAsset(resolvedRel) {
	if (copiedAssets.has(resolvedRel)) return true;
	const src = join(DOCS, resolvedRel);
	if (!existsSync(src)) return false;
	const dest = join(OUT_PUBLIC, resolvedRel);
	mkdirSync(dirname(dest), { recursive: true });
	cpSync(src, dest);
	copiedAssets.add(resolvedRel);
	return true;
}

// Build the environment the pure rewrite transform needs for one source file.
// A fresh ctx per file closes over `sourceRel`, so resolveMd/copyImage attribute
// an unmapped link or missing image to the doc it appears in without a
// module-scoped "current file" that every caller has to remember to set.
// `imageRemote` is empty so images are copied locally (offline bundle) rather
// than pointed at a remote URL.
function createRewriteCtx(sourceRel) {
	return {
		imageRemote: "",
		resolveMd(resolvedRel) {
			const mapped = fileMap.get(resolvedRel);
			if (mapped) return routeHref(mapped.route);
			unmappedLinks.push({ source: sourceRel, target: resolvedRel });
			return null;
		},
		copyImage(resolvedRel) {
			if (copyAsset(resolvedRel)) return `/${resolvedRel}`;
			unresolvedImages.push({ source: sourceRel, target: resolvedRel });
			return null;
		},
		// A repo path outside the docs/ corpus (e.g. a `../../coderd` source-tree
		// link). It can never resolve to a synced page or a bundled asset, so
		// point it at the file or directory on GitHub rather than leave a relative
		// path that 404s in the offline bundle. Use `/blob/` for a path whose last
		// segment looks like a file (has a dot) and `/tree/` for a directory;
		// GitHub redirects between the two, so this only avoids a needless
		// redirect.
		sourceLink(repoRel) {
			const base = repoRel.slice(repoRel.lastIndexOf("/") + 1);
			const kind = base.includes(".") ? "blob" : "tree";
			return `https://github.com/${SOURCE_REPO}/${kind}/${SOURCE_REF}/${repoRel}`;
		},
	};
}

rmSync(OUT_CONTENT, { recursive: true, force: true });
mkdirSync(OUT_CONTENT, { recursive: true });
// Keep the tracked placeholder so a clean checkout still has the source dir
// (and the working tree stays clean for CI's unstaged-file check).
writeFileSync(join(OUT_CONTENT, ".gitkeep"), "");

// Drop fumadocs-mdx's stale typed source so the next `fumadocs-mdx` / `next
// build` rebuilds it from the content written below. fumadocs-mdx skips
// regeneration when .source is newer than the content it indexes, and the
// `postinstall: fumadocs-mdx` hook builds .source once against the still-empty
// content/docs before this sync runs. That stub can end up newer than the
// freshly synced content, so without this fumadocs-mdx no-ops and ships an
// empty typed source (tsc: ".source/server.ts is not a module").
rmSync(DOT_SOURCE, { recursive: true, force: true });

function yamlString(s) {
	return JSON.stringify(String(s));
}

let written = 0;
for (const rel of manifestMd) {
	const { outRel, route } = fileMap.get(rel);
	const raw = readFileSync(join(DOCS, rel), "utf8");

	const meta = manifestMeta.get(route);
	const {
		title: extractedTitle,
		h1Line,
		frontmatterEnd,
		description: frontmatterDescription,
	} = extractTitle(raw, route, manifestMeta);
	// The manifest's first route (the README homepage, route "") is titled
	// "About" so coder.com's static landing page keeps working, but coder.com
	// itself renders that root entry as "Home" in the sidebar and derives a
	// separate "About" section from its children. Mirror that here so the offline
	// sidebar shows a single "Home" landing instead of a second "About" entry that
	// collides with the About folder (screenshots, support, contributing).
	const title = route === "" ? "Home" : extractedTitle;
	// Strip the leading frontmatter block (if any) and the body H1 in one pass,
	// so a make gen file's `---`/`title:`/`---` header never survives into the
	// emitted body and the H1 is not duplicated below the injected frontmatter.
	let body = raw;
	if (frontmatterEnd > 0 || h1Line >= 0) {
		const lines = raw.split("\n");
		const drop = new Set();
		for (let i = 0; i < frontmatterEnd; i++) drop.add(i);
		if (h1Line >= 0) {
			drop.add(h1Line);
			let after = h1Line + 1;
			if (lines[after] === "") {
				drop.add(after);
				after++;
			}
			// A decorative `---` thematic break directly under the stripped H1 (a
			// heading/body separator in the source) would otherwise become the first
			// body line and render as a stray <hr> under the injected frontmatter.
			// Drop that single rule line; real body content is untouched.
			if (/^---\s*$/.test(lines[after] ?? "")) drop.add(after);
		}
		body = lines.filter((_, i) => !drop.has(i)).join("\n");
	}
	const stripped = stripHtmlComments(body);
	body = stripped.content;
	if (stripped.unclosedCommentLine !== null) {
		unclosedComments.push({ source: rel, line: stripped.unclosedCommentLine });
	}
	body = rewriteContent(body, rel, createRewriteCtx(rel));
	body = normalizeFences(body);
	body = normalizeStepHeadings(body);
	body = normalizeAngleBrackets(body);
	body = escapeCurlyBraces(body);
	body = selfCloseVoidElements(body);

	// Prefer a description from the source's own frontmatter (make gen CLI pages
	// carry one), then the manifest's.
	const description = frontmatterDescription || meta?.description;
	const fm = [`title: ${yamlString(title)}`];
	if (description) fm.push(`description: ${yamlString(description)}`);
	const out = `---\n${fm.join("\n")}\n---\n\n${body.replace(/^\n+/, "")}`;

	const dest = join(OUT_CONTENT, outRel);
	mkdirSync(dirname(dest), { recursive: true });
	writeFileSync(dest, out);
	written++;
}

// Copy the whole images tree so every referenced image is present even if a
// reference is not caught by the link rewriter (matches the previous bundle,
// which shipped all of docs/images).
rmSync(join(OUT_PUBLIC, "images"), { recursive: true, force: true });
if (existsSync(join(DOCS, "images"))) {
	cpSync(join(DOCS, "images"), join(OUT_PUBLIC, "images"), { recursive: true });
}

// pass 4: meta.json per directory
const dirModel = buildDirModel(
	manifestMd.map((rel) => fileMap.get(rel).outRel),
);
const metas = buildMeta(dirModel, {
	routeOrder,
	childOrderByDir,
	manifestMeta,
});
let metaFilesWritten = 0;
for (const { dir, title, pages } of metas) {
	const metaObj = {};
	if (title) metaObj.title = title;
	metaObj.pages = pages;
	const dest = join(OUT_CONTENT, dir, "meta.json");
	mkdirSync(dirname(dest), { recursive: true });
	writeFileSync(dest, `${JSON.stringify(metaObj, null, 2)}\n`);
	metaFilesWritten++;
}

console.log(
	`[sync-docs] pages=${written} ` +
		`(skipped ${allMd.length - manifestMd.length} non-manifest), ` +
		`meta.json=${metaFilesWritten}, ` +
		`images=${copiedAssets.size} referenced (+ full images/ tree), ` +
		`unmapped .md links=${unmappedLinks.length}, ` +
		`unresolved images=${unresolvedImages.length}`,
);

// Fail the sync (and therefore the offlinedocs build and the release target) on
// any defect that would otherwise ship silently: a dead inter-doc link, a broken
// image reference, an unterminated HTML comment that blanks a page, two source
// files whose routes collide and overwrite one another, or a manifest route with
// no backing file. Each is named with its source so a docs change that
// introduces one is actionable in CI instead of shipping.
if (
	unmappedLinks.length > 0 ||
	unresolvedImages.length > 0 ||
	unclosedComments.length > 0 ||
	outputCollisions.length > 0 ||
	missingManifestRoutes.length > 0
) {
	if (unmappedLinks.length > 0) {
		console.error(
			`\n[sync-docs] ERROR: ${unmappedLinks.length} inter-doc link(s) do not ` +
				"resolve to a synced page and would ship as dead links:",
		);
		for (const { source, target } of unmappedLinks) {
			console.error(`  ${source} -> ${target}`);
		}
		console.error(
			"\nFix each link to point at a manifest page, or use an absolute URL " +
				"(for example a https://github.com/... link) when the target is " +
				"intentionally outside the published docs corpus.",
		);
	}
	if (unresolvedImages.length > 0) {
		console.error(
			`\n[sync-docs] ERROR: ${unresolvedImages.length} image reference(s) do ` +
				"not resolve to a file under docs/ and would ship as broken images:",
		);
		for (const { source, target } of unresolvedImages) {
			console.error(`  ${source} -> ${target}`);
		}
		console.error(
			"\nFix each image path to point at an existing file under docs/, or " +
				"remove the reference.",
		);
	}
	if (unclosedComments.length > 0) {
		console.error(
			`\n[sync-docs] ERROR: ${unclosedComments.length} HTML comment(s) open ` +
				"with `<!--` but never close, which would blank the rest of the page:",
		);
		for (const { source, line } of unclosedComments) {
			console.error(`  ${source}:${line}`);
		}
		console.error(
			"\nClose each comment with `-->`, or remove the stray `<!--`.",
		);
	}
	if (outputCollisions.length > 0) {
		console.error(
			`\n[sync-docs] ERROR: ${outputCollisions.length} output path(s) are ` +
				"claimed by more than one source file, so one page would overwrite " +
				"the other:",
		);
		for (const { outRel, sources } of outputCollisions) {
			console.error(`  ${outRel} <- ${sources.join(", ")}`);
		}
		console.error(
			"\nRename one of the colliding files so their routes differ (slugs are " +
				"case- and separator-insensitive).",
		);
	}
	if (missingManifestRoutes.length > 0) {
		console.error(
			`\n[sync-docs] ERROR: ${missingManifestRoutes.length} manifest ` +
				"route(s) name a doc with no backing file, so they would be dropped " +
				"from the sidebar with no page:",
		);
		for (const { route, path } of missingManifestRoutes) {
			console.error(`  ${route || "(root)"} <- ${path ?? "(no path)"}`);
		}
		console.error(
			"\nRestore the missing file, or remove the entry from docs/manifest.json.",
		);
	}
	process.exit(1);
}
