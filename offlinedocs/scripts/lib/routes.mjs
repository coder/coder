/*
 * Pure route-mapping logic for sync-docs.mjs.
 *
 * Turning docs-relative markdown paths and docs/manifest.json into output
 * routes, an ordering model, and the per-directory meta.json page lists is pure
 * over its inputs (a path, the manifest, a set of directory routes), so it lives
 * here, next to lib/transform.mjs, with no filesystem or network access and no
 * top-level side effects. sync-docs.mjs keeps the I/O and calls these builders,
 * so the route logic (where a slug collision or a manifest reorder can silently
 * drop or misorder a page) can be unit-tested without running the sync.
 */
import {
	isIndexFile,
	lastSeg,
	normalizeManifestPath,
	slugSegment,
	titleCase,
} from "./transform.mjs";

// Every directory route implied by the corpus: for docs/a/b/c.md the routes
// "a" and "a/b". A file whose own route equals one of these is emitted as that
// directory's index.md rather than a sibling page, so the route and the
// directory do not both try to own the same URL.
export function buildDirRoutes(allMd) {
	const dirRoutes = new Set();
	for (const rel of allMd) {
		const parts = rel.split("/");
		parts.pop();
		const dirSlugs = parts.map(slugSegment);
		for (let i = 1; i <= dirSlugs.length; i++) {
			dirRoutes.add(dirSlugs.slice(0, i).join("/"));
		}
	}
	return dirRoutes;
}

// Map a docs-relative markdown path to its output file (`outRel`) and `route`.
// index/readme files become the directory index; a non-index file whose route
// collides with a directory route also becomes that directory's index.md.
export function mapMdPath(relPath, dirRoutes) {
	const parts = relPath.split("/");
	const base = parts.pop();
	const dirSlugs = parts.map(slugSegment);
	if (isIndexFile(base)) {
		const route = dirSlugs.join("/");
		const outRel = `${route ? `${route}/` : ""}index.md`;
		return { outRel, route };
	}
	const route = [...dirSlugs, slugSegment(base)].join("/");
	if (dirRoutes.has(route)) {
		return { outRel: `${route}/index.md`, route };
	}
	return { outRel: `${route}.md`, route };
}

// Build `rel -> { outRel, route }` for every markdown file, and detect distinct
// source files that map to the same output path. A collision (two basenames
// that slugify the same, a case-only difference, or a file whose route collides
// with a directory index) silently overwrites one published page with another,
// so each is returned as `{ outRel, sources: [relA, relB, ...] }` for the caller
// to fail on instead of shipping the loss.
export function buildFileMap(allMd, dirRoutes) {
	const fileMap = new Map();
	const byOut = new Map();
	for (const rel of allMd) {
		const mapped = mapMdPath(rel, dirRoutes);
		fileMap.set(rel, mapped);
		const sources = byOut.get(mapped.outRel);
		if (sources) sources.push(rel);
		else byOut.set(mapped.outRel, [rel]);
	}
	const collisions = [];
	for (const [outRel, sources] of byOut) {
		if (sources.length > 1) collisions.push({ outRel, sources });
	}
	return { fileMap, collisions };
}

// Resolve a manifest node's `path` to its route, or null when the node has no
// path (a pure grouping node).
export function manifestRoute(node, dirRoutes) {
	return node.path
		? mapMdPath(normalizeManifestPath(node.path), dirRoutes).route
		: null;
}

// Walk the manifest tree once, deriving three views used downstream:
//   manifestMeta     route -> { title, description }
//   routeOrder       route -> first-seen index (the manifest's document order)
//   childOrderByDir  directory route -> its child routes, in manifest order
// A route that appears more than once (the manifest lists a page twice) keeps
// its first metadata and order; later occurrences are ignored. This mirrors the
// sync's historical tolerance of duplicate manifest entries rather than failing
// on them.
export function buildManifestModel(manifest, dirRoutes) {
	const manifestMeta = new Map();
	const routeOrder = new Map();
	const childOrderByDir = new Map();
	let order = 0;

	function walk(nodes) {
		for (const node of nodes || []) {
			const r = manifestRoute(node, dirRoutes);
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
					.map((child) => manifestRoute(child, dirRoutes))
					.filter((x) => x !== null);
				if (r !== null && r !== "") childOrderByDir.set(r, childRoutes);
				walk(node.children);
			}
		}
	}
	walk(manifest.routes);

	const rootOrder = (manifest.routes || [])
		.map((node) => manifestRoute(node, dirRoutes))
		.filter((x) => x !== null);
	childOrderByDir.set("", rootOrder);

	return { manifestMeta, routeOrder, childOrderByDir };
}

// Build the directory model (the files and subdirectories under each directory
// route) for the output paths that will actually be written.
export function buildDirModel(outRels) {
	const dirModel = new Map();
	function ensureDir(d) {
		if (!dirModel.has(d)) {
			dirModel.set(d, { files: new Set(), subdirs: new Set() });
		}
		return dirModel.get(d);
	}
	ensureDir("");
	for (const outRel of outRels) {
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
	return dirModel;
}

// Lowest manifest order of `route` or anything beneath it, so a directory sorts
// by the earliest-ordered page it contains. Infinity when nothing under it is
// ordered.
function minOrderUnder(route, routeOrder) {
	let best = Number.POSITIVE_INFINITY;
	for (const [r, o] of routeOrder) {
		if (r === route || r.startsWith(`${route}/`)) best = Math.min(best, o);
	}
	return best;
}

// Sort key for a meta.json entry, as a tuple compared left to right:
//   bucket 0: the item is in the directory's manifest child order -> that index
//   bucket 1: not listed, but ordered by its own manifest position -> that order
//   bucket 2: unordered -> falls to the end, broken by name
// Tuples keep listed items ahead of unlisted ones without the two large sentinel
// numbers a single numeric key needed, so there is no corpus-size ceiling.
function sortKey(item, childOrder, routeOrder) {
	const pos = childOrder.indexOf(item.route);
	if (pos >= 0) return [0, pos];
	const ord = item.isDir
		? minOrderUnder(item.route, routeOrder)
		: routeOrder.get(item.route);
	if (ord !== undefined && ord !== Number.POSITIVE_INFINITY) return [1, ord];
	return [2, 0];
}

// Compute the meta.json objects for every directory: a `title` (null for the
// root) and the ordered `pages` list. Subdirectories and files are interleaved
// and ordered by sortKey, then alphabetically; the root gets `index` first.
export function buildMeta(
	dirModel,
	{ routeOrder, childOrderByDir, manifestMeta },
) {
	const metas = [];
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
		items.sort((a, b) => {
			const ka = sortKey(a, childOrder, routeOrder);
			const kb = sortKey(b, childOrder, routeOrder);
			return ka[0] - kb[0] || ka[1] - kb[1] || a.name.localeCompare(b.name);
		});

		const pages = items.map((i) => i.name);
		if (dir === "") pages.unshift("index");

		const title =
			dir === ""
				? null
				: manifestMeta.get(dir)?.title || titleCase(lastSeg(dir));
		metas.push({ dir, title, pages });
	}
	return metas;
}
