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
 * The pure string transforms live in ./lib/transform.mjs so they can be tested
 * in isolation.
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
	escapeCurlyBraces,
	extractTitle,
	isIndexFile,
	lastSeg,
	normalizeAngleBrackets,
	normalizeFences,
	normalizeManifestPath,
	normalizeStepHeadings,
	rewriteContent,
	selfCloseVoidElements,
	slugSegment,
	stripHtmlComments,
	titleCase,
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

if (!existsSync(join(DOCS, "manifest.json"))) {
	console.error(
		`[sync-docs] no docs manifest at ${join(DOCS, "manifest.json")}`,
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

// pass 1: collect every markdown file
const allMd = walk(DOCS)
	.map((f) => relative(DOCS, f).split("\\").join("/"))
	.filter((rel) => rel.endsWith(".md"))
	.filter((rel) => !rel.startsWith(".style/"));

const dirRoutes = new Set();
for (const rel of allMd) {
	const parts = rel.split("/");
	parts.pop();
	const dirSlugs = parts.map(slugSegment);
	for (let i = 1; i <= dirSlugs.length; i++) {
		dirRoutes.add(dirSlugs.slice(0, i).join("/"));
	}
}

function mapMdPath(relPath) {
	const parts = relPath.split("/");
	const base = parts.pop();
	const dirSlugs = parts.map(slugSegment);
	if (isIndexFile(base)) {
		const route = dirSlugs.join("/");
		const outRel = (route ? `${route}/` : "") + "index.md";
		return { outRel, route };
	}
	const route = [...dirSlugs, slugSegment(base)].join("/");
	if (dirRoutes.has(route)) {
		return { outRel: `${route}/index.md`, route };
	}
	return { outRel: `${route}.md`, route };
}

const fileMap = new Map();
for (const rel of allMd) {
	fileMap.set(rel, mapMdPath(rel));
}

// pass 2: manifest (titles, descriptions, ordering)
const manifest = JSON.parse(readFileSync(join(DOCS, "manifest.json"), "utf8"));
const manifestMeta = new Map();
const routeOrder = new Map();
const childOrderByDir = new Map();
let order = 0;

function manifestRoute(node) {
	return node.path ? mapMdPath(normalizeManifestPath(node.path)).route : null;
}

function walkManifest(nodes) {
	for (const node of nodes || []) {
		const r = manifestRoute(node);
		if (r !== null) {
			if (!manifestMeta.has(r)) {
				manifestMeta.set(r, {
					title: node.title,
					description: node.description,
				});
			}
			if (!routeOrder.has(r)) routeOrder.set(r, order++);
		}
		if (node.children && node.children.length) {
			const childRoutes = node.children
				.map(manifestRoute)
				.filter((x) => x !== null);
			if (r !== null && r !== "") childOrderByDir.set(r, childRoutes);
			walkManifest(node.children);
		}
	}
}
walkManifest(manifest.routes);

const rootOrder = (manifest.routes || [])
	.map(manifestRoute)
	.filter((x) => x !== null);
childOrderByDir.set("", rootOrder);

// Restrict output to pages referenced by the manifest so the generated sidebar
// matches docs/manifest.json exactly. Files that exist under docs/ but are not
// in the manifest (orphans) are intentionally excluded; the previous
// offlinedocs renderer likewise only rendered manifest routes.
const manifestMd = allMd.filter((rel) =>
	routeOrder.has(fileMap.get(rel).route),
);

// pass 3: rewrite + write content
const copiedAssets = new Set();
let unmappedMdLinks = 0;

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

// Environment the pure rewrite transform needs. `imageRemote` is empty so
// images are copied locally (offline bundle) rather than pointed at a remote URL.
const rewriteCtx = {
	imageRemote: "",
	resolveMd(resolvedRel) {
		const mapped = fileMap.get(resolvedRel);
		if (mapped) return routeHref(mapped.route);
		unmappedMdLinks++;
		return null;
	},
	copyImage(resolvedRel) {
		return copyAsset(resolvedRel) ? `/${resolvedRel}` : null;
	},
};

rmSync(OUT_CONTENT, { recursive: true, force: true });
mkdirSync(OUT_CONTENT, { recursive: true });
// Keep the tracked placeholder so a clean checkout still has the source dir
// (and the working tree stays clean for CI's unstaged-file check).
writeFileSync(join(OUT_CONTENT, ".gitkeep"), "");

function yamlString(s) {
	return JSON.stringify(String(s));
}

let written = 0;
for (const rel of manifestMd) {
	const { outRel, route } = fileMap.get(rel);
	const raw = readFileSync(join(DOCS, rel), "utf8");

	const meta = manifestMeta.get(route);
	const { title: manifestTitle, h1Line } = extractTitle(
		raw,
		route,
		manifestMeta,
	);
	// The manifest's first route (the README homepage, route "") is titled
	// "About" so coder.com's static landing page keeps working, but coder.com
	// itself renders that root entry as "Home" in the sidebar and derives a
	// separate "About" section from its children. Mirror that here so the offline
	// sidebar shows a single "Home" landing instead of a second "About" entry that
	// collides with the About folder (screenshots, support, contributing).
	const title = route === "" ? "Home" : manifestTitle;
	let body = raw;
	if (h1Line >= 0) {
		const lines = raw.split("\n");
		lines.splice(h1Line, 1);
		if (lines[h1Line] === "") lines.splice(h1Line, 1);
		body = lines.join("\n");
	}
	body = stripHtmlComments(body);
	body = rewriteContent(body, rel, rewriteCtx);
	body = normalizeFences(body);
	body = normalizeStepHeadings(body);
	body = normalizeAngleBrackets(body);
	body = escapeCurlyBraces(body);
	body = selfCloseVoidElements(body);

	const fm = [`title: ${yamlString(title)}`];
	if (meta?.description)
		fm.push(`description: ${yamlString(meta.description)}`);
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
const dirModel = new Map();
function ensureDir(d) {
	if (!dirModel.has(d)) {
		dirModel.set(d, { files: new Set(), subdirs: new Set() });
	}
	return dirModel.get(d);
}
ensureDir("");
for (const rel of manifestMd) {
	const { outRel } = fileMap.get(rel);
	const parts = outRel.split("/");
	const base = parts.pop();
	let prev = "";
	for (let i = 0; i < parts.length; i++) {
		const cur = parts.slice(0, i + 1).join("/");
		ensureDir(cur);
		ensureDir(prev).subdirs.add(parts[i]);
		prev = cur;
	}
	if (!/^index\.md$/i.test(base)) {
		ensureDir(parts.join("/")).files.add(base.replace(/\.md$/, ""));
	}
}

function minOrderUnder(route) {
	let best = Infinity;
	for (const [r, o] of routeOrder) {
		if (r === route || r.startsWith(`${route}/`)) best = Math.min(best, o);
	}
	return best;
}

let metaFilesWritten = 0;
for (const [dir, model] of dirModel) {
	const childOrder = childOrderByDir.get(dir) || [];
	const items = [];
	for (const name of model.subdirs) {
		items.push({
			name,
			route: dir === "" ? name : `${dir}/${name}`,
			isDir: true,
		});
	}
	for (const name of model.files) {
		items.push({
			name,
			route: dir === "" ? name : `${dir}/${name}`,
			isDir: false,
		});
	}

	function keyFor(it) {
		const pos = childOrder.indexOf(it.route);
		if (pos >= 0) return pos;
		const ord = it.isDir ? minOrderUnder(it.route) : routeOrder.get(it.route);
		return 100000 + (ord === undefined || ord === Infinity ? 99999 : ord);
	}
	items.sort((a, b) => keyFor(a) - keyFor(b) || a.name.localeCompare(b.name));

	const pages = items.map((i) => i.name);
	if (dir === "") pages.unshift("index");

	const metaObj = {};
	if (dir !== "") {
		const t = manifestMeta.get(dir)?.title || titleCase(lastSeg(dir));
		if (t) metaObj.title = t;
	}
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
		`unmapped .md links=${unmappedMdLinks}`,
);
