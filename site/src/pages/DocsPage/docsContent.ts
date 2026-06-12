// Helpers for rendering the version-matched documentation that the site
// build copies into static/docs-content/ (see site/scripts/copyDocsContent.sh).
// Route-mapping rules mirror offlinedocs: index files map to their
// directory, other files map to their path without the .md extension.

export interface DocsManifestRoute {
	title: string;
	description?: string;
	path: string;
	icon_path?: string;
	children?: readonly DocsManifestRoute[];
}

export interface DocsManifest {
	versions: readonly string[];
	routes: readonly DocsManifestRoute[];
}

export function manifestPathToRoute(filePath: string): string {
	let route = filePath.replace(/^\.\//, "").replace(/\.md$/, "");
	if (route === "README") {
		return "";
	}
	if (route.endsWith("/index")) {
		route = route.slice(0, -"/index".length);
	}
	return route;
}

export interface DocsRouteMaps {
	// Route path ("install/airgap") to docs-relative file ("install/airgap.md").
	routeToFile: Map<string, string>;
	// Docs-relative file to route path, for rewriting relative markdown links.
	fileToRoute: Map<string, string>;
	// Route path to the manifest title, used for the document title.
	routeToTitle: Map<string, string>;
}

export function buildRouteMaps(manifest: DocsManifest): DocsRouteMaps {
	const routeToFile = new Map<string, string>();
	const fileToRoute = new Map<string, string>();
	const routeToTitle = new Map<string, string>();
	const visit = (routes: readonly DocsManifestRoute[]) => {
		for (const manifestRoute of routes) {
			const file = manifestRoute.path.replace(/^\.\//, "");
			const route = manifestPathToRoute(manifestRoute.path);
			// The manifest lists some pages under multiple sections; the first
			// occurrence wins so generated routes stay stable.
			if (!routeToFile.has(route)) {
				routeToFile.set(route, file);
				routeToTitle.set(route, manifestRoute.title);
			}
			if (!fileToRoute.has(file)) {
				fileToRoute.set(file, route);
			}
			if (manifestRoute.children) {
				visit(manifestRoute.children);
			}
		}
	};
	visit(manifest.routes);
	return { routeToFile, fileToRoute, routeToTitle };
}

interface ResolvedDocLink {
	// Path relative to the docs/ directory, e.g. "install/airgap.md".
	path: string;
	// Anchor including the leading "#", or "".
	hash: string;
}

// Resolves a relative href found in currentFile against the docs tree.
// Returns null for absolute URLs, protocol links, root-relative paths,
// and same-page anchors, which should be left unchanged.
export function resolveRelativeDocLink(
	currentFile: string,
	href: string,
): ResolvedDocLink | null {
	if (
		/^[a-z][a-z0-9+.-]*:/i.test(href) ||
		href.startsWith("//") ||
		href.startsWith("/") ||
		href.startsWith("#")
	) {
		return null;
	}
	// Use URL resolution semantics against a synthetic origin so ".."
	// segments normalize correctly.
	try {
		const url = new URL(href, `https://docs.invalid/${currentFile}`);
		return {
			path: decodeURIComponent(url.pathname.replace(/^\//, "")),
			hash: url.hash,
		};
	} catch {
		// Malformed hrefs, for example a literal "%" that is not a valid
		// escape sequence, are left unchanged rather than crashing the
		// renderer.
		return null;
	}
}

// Builds a raw.githubusercontent.com URL for a docs asset, pinned to the
// git tag matching this build. Uses the same version normalization as
// utils/docs.ts: strip build metadata after "+", strip a "-devel" suffix,
// and keep "-rc.X" suffixes. Dev builds (v0.0.0) fall back to main.
export function githubDocsAssetUrl(
	docsRelativePath: string,
	version: string | undefined,
): string {
	let ref = "main";
	if (version) {
		const normalized = version.split("+")[0].replace(/-devel$/, "");
		if (normalized !== "v0.0.0") {
			ref = normalized;
		}
	}
	return `https://raw.githubusercontent.com/coder/coder/${ref}/docs/${docsRelativePath}`;
}

export const docsManifestQuery = () => ({
	queryKey: ["docs", "manifest"],
	queryFn: async (): Promise<DocsManifest> => {
		const res = await fetch("/docs-content/manifest.json");
		if (!res.ok) {
			throw new Error(`Failed to load docs manifest: HTTP ${res.status}`);
		}
		return res.json();
	},
	staleTime: Number.POSITIVE_INFINITY,
	retry: false,
});

export const docsPageQuery = (filePath: string) => ({
	queryKey: ["docs", "page", filePath],
	queryFn: async (): Promise<string> => {
		const res = await fetch(`/docs-content/${filePath}`);
		if (!res.ok) {
			throw new Error(`Failed to load docs page: HTTP ${res.status}`);
		}
		const contentType = res.headers.get("Content-Type") ?? "";
		if (contentType.includes("text/html")) {
			// The SPA fallback serves index.html with a 200 status for
			// unknown paths, so an HTML response means the markdown file
			// does not exist.
			throw new Error(`Docs page not found: ${filePath}`);
		}
		return res.text();
	},
	staleTime: Number.POSITIVE_INFINITY,
	retry: false,
});
