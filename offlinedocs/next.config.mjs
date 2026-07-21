import { createMDX } from "fumadocs-mdx/next";

const withMDX = createMDX();

/** @type {import('next').NextConfig} */
const config = {
	// Pin the workspace root to this folder. offlinedocs has its own
	// package.json + lockfile inside the coder/coder repo, and Next would
	// otherwise infer the repo root (which also has a lockfile) as the root.
	turbopack: {
		root: import.meta.dirname,
	},
	// Emit a fully static site into out/ so the docs bundle is self-contained
	// and can be served by any static file host with no Node server. This is
	// what the release pipeline tars into coder_docs_<version>.tgz for
	// offline/airgapped use.
	output: "export",
	reactStrictMode: true,
	// Canonical URLs end in a slash and every route is emitted as
	// <route>/index.html, which is what a plain static file server expects.
	trailingSlash: true,
	// Server source maps are large and unnecessary for a shipped static bundle.
	productionBrowserSourceMaps: false,
	// Doc images are copied into the bundle (see scripts/sync-docs.mjs) and
	// served as-is; next/image optimization needs a running server, which a
	// static export does not have.
	images: {
		unoptimized: true,
	},
};

export default withMDX(config);
