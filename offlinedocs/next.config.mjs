// Next.js configuration for the offlinedocs static export. It wires the Fumadocs
// MDX plugin into the build (createMDX/withMDX) and produces the
// coder_docs_<version>.tgz artifact the release pipeline ships for offline use.
// Each build setting is documented at its field below.

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
	//
	// Only export for production builds (`next build`). Under `output: export`
	// an optional catch-all route rejects any path that generateStaticParams
	// did not emit, so `next dev` throws on incidental requests (a stale
	// /serviceWorker.js, favicon probes, etc.). Leaving dev on the default
	// server keeps the local workflow normal (clean 404s) while `next build`
	// still produces the static export.
	output: process.env.NODE_ENV === "production" ? "export" : undefined,
	reactStrictMode: true,
	// Canonical URLs end in a slash and every route is emitted as
	// <route>/index.html, which is what a plain static file server expects.
	trailingSlash: true,
	// Doc images are copied into the bundle (refer to scripts/sync-docs.mjs)
	// and served as-is; next/image optimization needs a running server,
	// which a static export does not have.
	images: {
		unoptimized: true,
	},
};

export default withMDX(config);
