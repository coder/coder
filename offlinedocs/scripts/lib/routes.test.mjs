import assert from "node:assert/strict";
import test from "node:test";

import {
	buildDirModel,
	buildDirRoutes,
	buildFileMap,
	buildManifestModel,
	buildMeta,
	manifestRoute,
	mapMdPath,
} from "./routes.mjs";

test("mapMdPath maps index/readme and plain files to routes", () => {
	const dirRoutes = buildDirRoutes(["a/index.md", "a/b.md"]);
	// An index file becomes the directory index.
	assert.deepEqual(mapMdPath("a/index.md", dirRoutes), {
		outRel: "a/index.md",
		route: "a",
	});
	// A plain file becomes route.md.
	assert.deepEqual(mapMdPath("a/b.md", dirRoutes), {
		outRel: "a/b.md",
		route: "a/b",
	});
	// The root README maps to the empty route.
	assert.deepEqual(mapMdPath("README.md", buildDirRoutes(["README.md"])), {
		outRel: "index.md",
		route: "",
	});
});

test("mapMdPath emits index.md when a file's route collides with a directory", () => {
	// docs/kubernetes.md alongside docs/kubernetes/*.md: the file owns the
	// directory's index rather than trying to be a sibling kubernetes.md.
	const dirRoutes = buildDirRoutes(["kubernetes.md", "kubernetes/aks.md"]);
	assert.deepEqual(mapMdPath("kubernetes.md", dirRoutes), {
		outRel: "kubernetes/index.md",
		route: "kubernetes",
	});
});

test("manifestRoute maps a node path and returns null for grouping nodes", () => {
	const dirRoutes = buildDirRoutes(["a/b.md"]);
	assert.equal(manifestRoute({ path: "./a/b.md" }, dirRoutes), "a/b");
	assert.equal(manifestRoute({ title: "Group" }, dirRoutes), null);
});

test("buildFileMap flags distinct sources that slugify to one output path", () => {
	// Separator-only (foo-bar vs foo_bar) and case-only differences collapse to
	// the same route and would silently overwrite each other.
	const allMd = ["foo-bar.md", "foo_bar.md", "unique.md"];
	const { fileMap, collisions } = buildFileMap(allMd, buildDirRoutes(allMd));
	assert.equal(fileMap.get("foo-bar.md").outRel, "foo-bar.md");
	assert.equal(collisions.length, 1);
	assert.equal(collisions[0].outRel, "foo-bar.md");
	assert.deepEqual([...collisions[0].sources].sort(), [
		"foo-bar.md",
		"foo_bar.md",
	]);
});

test("buildFileMap reports no collision for distinct routes", () => {
	const allMd = ["a.md", "b.md", "sub/a.md"];
	const { collisions } = buildFileMap(allMd, buildDirRoutes(allMd));
	assert.equal(collisions.length, 0);
});

test("buildManifestModel derives order from the manifest, not the filesystem", () => {
	const allMd = ["alpha.md", "beta.md"];
	const dirRoutes = buildDirRoutes(allMd);
	// The manifest lists beta before alpha; order must follow the manifest even
	// though alpha sorts first on disk.
	const manifest = {
		routes: [
			{ title: "Beta", path: "beta.md" },
			{ title: "Alpha", path: "alpha.md" },
		],
	};
	const { routeOrder, manifestMeta } = buildManifestModel(manifest, dirRoutes);
	assert.equal(routeOrder.get("beta"), 0);
	assert.equal(routeOrder.get("alpha"), 1);
	assert.equal(manifestMeta.get("alpha").title, "Alpha");
});

test("buildManifestModel tolerates a duplicate manifest entry", () => {
	const allMd = ["dup.md"];
	const dirRoutes = buildDirRoutes(allMd);
	// The same path listed twice keeps its first-seen order and metadata rather
	// than crashing or double-counting (docs/manifest.json has such a duplicate).
	const manifest = {
		routes: [
			{ title: "First", path: "dup.md" },
			{ title: "Second", path: "dup.md" },
		],
	};
	const { routeOrder, manifestMeta } = buildManifestModel(manifest, dirRoutes);
	assert.equal(routeOrder.size, 1);
	assert.equal(routeOrder.get("dup"), 0);
	assert.equal(manifestMeta.get("dup").title, "First");
});

test("buildMeta orders children by manifest child order, then by name", () => {
	const allMd = ["g/index.md", "g/a.md", "g/b.md", "g/c.md"];
	const dirRoutes = buildDirRoutes(allMd);
	// The manifest lists the children out of alphabetical order (c, a, b); the
	// meta pages must follow the manifest order.
	const manifest = {
		routes: [
			{
				title: "G",
				path: "g/index.md",
				children: [
					{ title: "C", path: "g/c.md" },
					{ title: "A", path: "g/a.md" },
					{ title: "B", path: "g/b.md" },
				],
			},
		],
	};
	const model = buildManifestModel(manifest, dirRoutes);
	const dirModel = buildDirModel(
		allMd.map((rel) => mapMdPath(rel, dirRoutes).outRel),
	);
	const metas = buildMeta(dirModel, model);
	const g = metas.find((m) => m.dir === "g");
	assert.deepEqual(g.pages, ["c", "a", "b"]);
	assert.equal(g.title, "G");
});

test("buildMeta appends unlisted pages after listed ones, ordered by name", () => {
	const allMd = ["g/index.md", "g/a.md", "g/z.md"];
	const dirRoutes = buildDirRoutes(allMd);
	// Only g/z is listed as a child; g/a is a manifest page but not in g's child
	// list, so it falls to the unlisted bucket after g/z.
	const manifest = {
		routes: [
			{
				title: "G",
				path: "g/index.md",
				children: [{ title: "Z", path: "g/z.md" }],
			},
			{ title: "A", path: "g/a.md" },
		],
	};
	const model = buildManifestModel(manifest, dirRoutes);
	const dirModel = buildDirModel(
		allMd.map((rel) => mapMdPath(rel, dirRoutes).outRel),
	);
	const g = buildMeta(dirModel, model).find((m) => m.dir === "g");
	assert.deepEqual(g.pages, ["z", "a"]);
});
