// Pure route and ordering logic for the docs sync: path-to-route mapping,
// collision detection, and per-directory meta.json order. No I/O, no side effects.
import {
	isIndexFile,
	lastSeg,
	normalizeManifestPath,
	slugSegment,
	titleCase,
} from "./transform.mjs";

// Every directory route implied by the corpus (for docs/a/b/c.md: "a" and "a/b").
export function buildDirRoutes(allMd) {
	return new Set(
		allMd.flatMap((rel) => {
			const dirSlugs = rel.split("/").slice(0, -1).map(slugSegment);
			return dirSlugs.map((_, i) => dirSlugs.slice(0, i + 1).join("/"));
		}),
	);
}

// Map a docs-relative markdown path to its output file (outRel) and route.
// A file whose route equals a directory becomes that directory's index.md.
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

// Map every markdown file to { outRel, route } and return the output paths that
// more than one source maps to, as collisions for the caller to fail on.
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
	const collisions = [...byOut]
		.filter(([, sources]) => sources.length > 1)
		.map(([outRel, sources]) => ({ outRel, sources }));
	return { fileMap, collisions };
}

// Resolve a manifest node's `path` to its route, or null when the node has no
// path (a pure grouping node).
export function manifestRoute(node, dirRoutes) {
	return node.path
		? mapMdPath(normalizeManifestPath(node.path), dirRoutes).route
		: null;
}

// Walk the manifest once, deriving route metadata, first-seen order, and
// per-directory child order. A route listed twice keeps its first occurrence.
export function buildManifestModel(manifest, dirRoutes) {
	const manifestMeta = new Map();
	const manifestPathByRoute = new Map();
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
					manifestPathByRoute.set(r, node.path);
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

	// Root children are the top-level dirs/files in the order their manifest
	// section introduces them. A section with no page of its own (the homepage-
	// mapped About, or a grouping node) contributes the dirs its subtree owns, so
	// about/ and support/ land at About's position instead of after every section.
	const seenRoot = new Set();
	const rootOrder = [];
	const pushRootSeg = (route) => {
		const seg = route ? route.split("/")[0] : "";
		if (seg && !seenRoot.has(seg)) {
			seenRoot.add(seg);
			rootOrder.push(seg);
		}
	};
	const collectRootSegs = (node) => {
		pushRootSeg(manifestRoute(node, dirRoutes));
		for (const child of node.children || []) collectRootSegs(child);
	};
	for (const node of manifest.routes || []) {
		const r = manifestRoute(node, dirRoutes);
		if (r) pushRootSeg(r);
		else collectRootSegs(node);
	}
	childOrderByDir.set("", rootOrder);

	return { manifestMeta, manifestPathByRoute, routeOrder, childOrderByDir };
}

// Manifest routes with no backing source file (deleted or renamed), returned
// for the caller to fail on instead of dropping them silently from the sidebar.
export function findUnbackedManifestRoutes(
	routeOrder,
	manifestPathByRoute,
	backedRoutes,
) {
	return [...routeOrder.keys()]
		.filter((route) => !backedRoutes.has(route))
		.map((route) => ({
			route,
			path: manifestPathByRoute.get(route) ?? null,
		}));
}

// The files and subdirectories under each directory route to be written.
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

// Lowest manifest order of route or anything beneath it, so a directory sorts by
// the earliest-ordered page it contains (Infinity when nothing under it is ordered).
function minOrderUnder(route, routeOrder) {
	return [...routeOrder]
		.filter(([r]) => r === route || r.startsWith(`${route}/`))
		.reduce((best, [, o]) => Math.min(best, o), Number.POSITIVE_INFINITY);
}

// Sort key tuple for a meta.json entry: bucket 0 = listed in manifest child order,
// 1 = ordered by own manifest position, 2 = unlisted (falls to the end by name).
function sortKey(item, childOrder, routeOrder) {
	const pos = childOrder.indexOf(item.route);
	if (pos >= 0) return [0, pos];
	const ord = item.isDir
		? minOrderUnder(item.route, routeOrder)
		: routeOrder.get(item.route);
	if (ord !== undefined && ord !== Number.POSITIVE_INFINITY) return [1, ord];
	return [2, 0];
}

// meta.json for every directory: a title (null at the root) and the ordered pages
// list, interleaving subdirs and files by sortKey then name; the root gets index first.
export function buildMeta(
	dirModel,
	{ routeOrder, childOrderByDir, manifestMeta },
) {
	const metas = [];
	for (const [dir, model] of dirModel) {
		const childOrder = childOrderByDir.get(dir) || [];
		const toItem = (isDir) => (name) => ({
			name,
			route: dir === "" ? name : `${dir}/${name}`,
			isDir,
		});
		const items = [
			...[...model.subdirs].map(toItem(true)),
			...[...model.files].map(toItem(false)),
		];
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
