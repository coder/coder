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
	// Next installs an optional catch-all route that rejects any path
	// generateStaticParams did not prerender. In dev that turns incidental
	// requests into hard errors instead of clean 404s - most notably a stale
	// /serviceWorker.js still registered at the dev origin by a prior site (a
	// local coder server or the old offlinedocs, both of which used :3000),
	// plus favicon probes and similar. When next dev errors on those, the page
	// renders but never becomes interactive; the symptom reported in UAT was
	// "nothing in the UI is clickable" (OS tabs, theme toggle, etc.). Gating
	// export to production keeps next dev on Next's normal server (clean 404s),
	// so local dev stays interactive, while next build still produces the full
	// static export.
	//
	// Running the docs anywhere: if a machine shows a dead or unclickable page
	// from an earlier session, a stale service worker is cached - unregister it
	// (DevTools > Application > Service workers > Unregister) and hard-reload.
	// The dev server also runs on :26337 (see the package.json scripts), not
	// :3000, to stay off that shared origin.
	output: process.env.NODE_ENV === "production" ? "export" : undefined,
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
