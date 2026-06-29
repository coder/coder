import { describe, expect, it, vi } from "vitest";
import {
	buildRouteMaps,
	type DocsManifest,
	docsPageQuery,
	githubDocsAssetUrl,
	manifestPathToRoute,
	resolveRelativeDocLink,
	stripHtmlComments,
} from "./docsContent";

describe("manifestPathToRoute", () => {
	it.each([
		["./README.md", ""],
		["./install/index.md", "install"],
		["./install/airgap.md", "install/airgap"],
		["./admin/networking/index.md", "admin/networking"],
	])("maps %s to %s", (filePath, route) => {
		expect(manifestPathToRoute(filePath)).toBe(route);
	});
});

describe("buildRouteMaps", () => {
	const manifest: DocsManifest = {
		versions: ["main"],
		routes: [
			{
				title: "About",
				path: "./README.md",
				children: [{ title: "Quickstart", path: "./tutorials/quickstart.md" }],
			},
			{
				title: "Install",
				path: "./install/index.md",
				children: [{ title: "Airgap", path: "./install/airgap.md" }],
			},
		],
	};

	it("maps routes to files, including nested children", () => {
		const { routeToFile } = buildRouteMaps(manifest);
		expect(routeToFile.get("")).toBe("README.md");
		expect(routeToFile.get("install")).toBe("install/index.md");
		expect(routeToFile.get("install/airgap")).toBe("install/airgap.md");
		expect(routeToFile.get("tutorials/quickstart")).toBe(
			"tutorials/quickstart.md",
		);
	});

	it("maps files back to routes", () => {
		const { fileToRoute } = buildRouteMaps(manifest);
		expect(fileToRoute.get("install/index.md")).toBe("install");
		expect(fileToRoute.get("README.md")).toBe("");
	});

	it("maps routes to manifest titles", () => {
		const { routeToTitle } = buildRouteMaps(manifest);
		expect(routeToTitle.get("install")).toBe("Install");
		expect(routeToTitle.get("")).toBe("About");
	});
});

describe("resolveRelativeDocLink", () => {
	it("resolves sibling links relative to the current file", () => {
		expect(resolveRelativeDocLink("install/index.md", "./airgap.md")).toEqual({
			path: "install/airgap.md",
			hash: "",
		});
	});

	it("resolves parent-directory links", () => {
		expect(
			resolveRelativeDocLink(
				"admin/networking/index.md",
				"../templates/foo.md",
			),
		).toEqual({ path: "admin/templates/foo.md", hash: "" });
	});

	it("preserves anchors", () => {
		expect(
			resolveRelativeDocLink("install/index.md", "./airgap.md#docs"),
		).toEqual({ path: "install/airgap.md", hash: "#docs" });
	});

	it("resolves image paths", () => {
		expect(
			resolveRelativeDocLink(
				"admin/setup/index.md",
				"../../images/admin/a.png",
			),
		).toEqual({ path: "images/admin/a.png", hash: "" });
	});

	it.each([
		"https://coder.com/docs",
		"mailto:support@coder.com",
		"//example.com/path",
		"/absolute/path",
		"#same-page-anchor",
	])("returns null for non-relative href %s", (href) => {
		expect(resolveRelativeDocLink("install/index.md", href)).toBeNull();
	});

	it("returns null for hrefs that fail URL parsing", () => {
		expect(
			resolveRelativeDocLink("install/index.md", "./50%-faster.md"),
		).toBeNull();
	});
});

describe("githubDocsAssetUrl", () => {
	it("pins release versions to their tag", () => {
		expect(githubDocsAssetUrl("images/a.png", "v2.24.2")).toBe(
			"https://raw.githubusercontent.com/coder/coder/v2.24.2/docs/images/a.png",
		);
	});

	it("strips build metadata, keeping non-devel versions pinned to their tag", () => {
		expect(githubDocsAssetUrl("images/a.png", "v2.25.0+abc123")).toBe(
			"https://raw.githubusercontent.com/coder/coder/v2.25.0/docs/images/a.png",
		);
	});

	it("uses main for devel builds regardless of the version prefix", () => {
		expect(githubDocsAssetUrl("images/a.png", "v2.26.0-devel+abc123")).toBe(
			"https://raw.githubusercontent.com/coder/coder/main/docs/images/a.png",
		);
		expect(githubDocsAssetUrl("images/a.png", "v0.0.0-devel+abc")).toBe(
			"https://raw.githubusercontent.com/coder/coder/main/docs/images/a.png",
		);
		expect(githubDocsAssetUrl("images/a.png", undefined)).toBe(
			"https://raw.githubusercontent.com/coder/coder/main/docs/images/a.png",
		);
	});

	it("preserves -rc suffixes", () => {
		expect(githubDocsAssetUrl("images/a.png", "v2.25.0-rc.1")).toBe(
			"https://raw.githubusercontent.com/coder/coder/v2.25.0-rc.1/docs/images/a.png",
		);
	});
});

describe("docsPageQuery", () => {
	it("rejects HTML responses from the SPA fallback", async () => {
		const originalFetch = globalThis.fetch;
		globalThis.fetch = vi.fn().mockResolvedValue(
			new Response("<!doctype html>", {
				status: 200,
				headers: { "Content-Type": "text/html; charset=utf-8" },
			}),
		);
		try {
			await expect(docsPageQuery("missing.md").queryFn()).rejects.toThrow(
				"Docs page not found: missing.md",
			);
		} finally {
			globalThis.fetch = originalFetch;
		}
	});

	it("strips HTML comments from successful responses", async () => {
		const originalFetch = globalThis.fetch;
		globalThis.fetch = vi.fn().mockResolvedValue(
			new Response("<!-- generated -->\n# Title\n", {
				status: 200,
				headers: { "Content-Type": "text/markdown; charset=utf-8" },
			}),
		);
		try {
			await expect(docsPageQuery("page.md").queryFn()).resolves.toBe(
				"\n# Title\n",
			);
		} finally {
			globalThis.fetch = originalFetch;
		}
	});
});

describe("stripHtmlComments", () => {
	it("removes single-line comments", () => {
		expect(stripHtmlComments("# Title\n<!-- hidden -->\nbody")).toBe(
			"# Title\n\nbody",
		);
	});

	it("removes multi-line comments", () => {
		expect(stripHtmlComments("a\n<!-- line 1\nline 2 -->\nb")).toBe("a\n\nb");
	});

	it("removes multiple comments in the same input", () => {
		expect(stripHtmlComments("<!-- one -->x<!-- two -->y")).toBe("xy");
	});

	it("leaves non-comment content untouched", () => {
		expect(stripHtmlComments("plain text")).toBe("plain text");
	});
});
