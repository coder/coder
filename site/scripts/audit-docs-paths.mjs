// Audit script for docs-URL drift.
//
// Cross-references docs(...) and string-literal docs-URL references in TS/TSX
// files against the source side of every /docs/* rule in
// coder.com/redirects.json. Anything that matches a redirect source is stale
// and needs to be updated to the redirect's destination.
//
// Usage:
//   node site/scripts/audit-docs-paths.mjs \
//     --redirects=/path/to/coder.com/redirects.json \
//     --roots=/path/to/coder/site,/path/to/coder.com/src \
//     --out=docs/.audit/redirects-audit-YYYY-MM-DD.md
//
// All flags are optional. Defaults assume a standard Coder dev layout under
// /home/coder/. The script never modifies source files; it only emits the
// report. The output file defaults to today's date so each run produces a
// dated snapshot.
//
// docs/.audit/ is gitignored; the report file is a local working artifact.

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";

// ---------------------------------------------------------------------------
// Redirect indexing.

export function loadRedirects(p) {
	return JSON.parse(fs.readFileSync(p, "utf-8"));
}

// Filter all entries to the /docs/* subset. Order matters because Next.js
// picks the first matching rule.
export function docsRedirects(all) {
	if (!Array.isArray(all)) {
		throw new TypeError(
			`docsRedirects: expected an array of redirect rules, got ${typeof all}`,
		);
	}
	return all.filter(
		(r) => typeof r.source === "string" && r.source.startsWith("/docs/"),
	);
}

// Match a path against a single redirect source. Returns the redirect's
// destination with path params substituted, or null if no match.
export function matchRedirect(refPath, redirect) {
	const src = redirect.source;
	const dst = redirect.destination;

	if (src === refPath) return dst;

	// Trailing /:path* wildcard (Next.js's "match anything below this prefix").
	if (src.endsWith("/:path*")) {
		const prefix = src.slice(0, -"/:path*".length);
		if (refPath === prefix) {
			return dst.endsWith("/:path*") ? dst.slice(0, -"/:path*".length) : dst;
		}
		if (refPath.startsWith(prefix + "/")) {
			const tail = refPath.slice(prefix.length); // includes leading slash
			if (dst.endsWith("/:path*")) {
				return dst.slice(0, -"/:path*".length) + tail;
			}
			return dst;
		}
	}

	// :slug(.*) at the end: same idea but params named "slug".
	if (src.endsWith(":slug(.*)")) {
		const prefix = src.slice(0, -":slug(.*)".length);
		if (refPath.startsWith(prefix)) {
			const tail = refPath.slice(prefix.length);
			if (dst.endsWith(":slug")) {
				return dst.slice(0, -":slug".length) + tail;
			}
			return dst;
		}
	}

	// :version capture groups appear in the new versioned redirects and are
	// rare elsewhere. The audit only cares whether a literal source path is
	// stale, so paths containing @version segments would never appear as
	// literals in TS/TSX. Skip.
	return null;
}

export function findMatchingRedirect(refPath, redirects) {
	for (const r of redirects) {
		const dst = matchRedirect(refPath, r);
		if (dst !== null) return { redirect: r, suggestedDestination: dst };
	}
	return null;
}

// ---------------------------------------------------------------------------
// Reference extraction.

// docs("/path") | docs('/path') | docs(`/path`) with NO ${expr}.
export const DOCS_LITERAL_RE = /\bdocs\(\s*(['"`])([^'"`)$]+)\1/g;

// docs(`/path/${expr}/more`). Captures the whole literal segment between the
// backticks so we can flag the literal prefix.
export const DOCS_TEMPLATE_RE = /\bdocs\(\s*`([^`]*\$\{[^`]*\}[^`]*)`/g;

// "https://coder.com/docs/..." wrapped in any string-literal delimiter.
// The backreference (\1) requires the closing delimiter to match the opening
// one, mirroring DOCS_LITERAL_RE.
export const HARDCODED_URL_RE =
	/(['"`])https?:\/\/(?:[a-z0-9-]+\.)?coder\.com(\/docs\/[^'"`)\s]+)\1/g;

// Markdown-link form: [text](https://coder.com/docs/...) or [text](/docs/...).
// Used inside notification bodies, doc strings, and other prose. The URL is
// bounded by ( and ), not by string delimiters.
export const MARKDOWN_LINK_RE =
	/\]\(\s*(?:https?:\/\/(?:[a-z0-9-]+\.)?coder\.com)?(\/docs\/[^\s)]+)\)/g;

export function stripQueryAndFragment(p) {
	// Remove hash fragment and query string before redirect matching.
	let out = p;
	const hash = out.indexOf("#");
	if (hash !== -1) out = out.slice(0, hash);
	const query = out.indexOf("?");
	if (query !== -1) out = out.slice(0, query);
	return out;
}

export function literalPrefix(tmpl) {
	// Return the literal portion before the first ${...} so we can do a
	// partial redirect match on the static prefix.
	const idx = tmpl.indexOf("${");
	return idx === -1 ? tmpl : tmpl.slice(0, idx);
}

// Map a regex match's 0-based string index to a 1-based line number using
// a precomputed array of line-start offsets.
function buildLineIndex(content) {
	const lineStarts = [0];
	for (let i = 0; i < content.length; i++) {
		if (content[i] === "\n") lineStarts.push(i + 1);
	}
	return lineStarts;
}

function indexToLine(idx, lineStarts) {
	// Binary search for the largest lineStart <= idx.
	let lo = 0;
	let hi = lineStarts.length - 1;
	while (lo < hi) {
		const mid = (lo + hi + 1) >>> 1;
		if (lineStarts[mid] <= idx) lo = mid;
		else hi = mid - 1;
	}
	return lo + 1; // 1-based
}

export function extractReferences(filePath, content) {
	const refs = [];
	const lineStarts = buildLineIndex(content);

	const push = (m, kind, rawArg, docsPath, dynamic) => {
		refs.push({
			file: filePath,
			lineNo: indexToLine(m.index, lineStarts),
			kind,
			rawArg,
			docsPath,
			dynamic,
		});
	};

	for (const m of content.matchAll(DOCS_LITERAL_RE)) {
		push(m, "docs-literal", m[2], "/docs" + stripQueryAndFragment(m[2]), false);
	}

	for (const m of content.matchAll(DOCS_TEMPLATE_RE)) {
		push(
			m,
			"docs-template",
			m[1],
			"/docs" + stripQueryAndFragment(literalPrefix(m[1])),
			true,
		);
	}

	for (const m of content.matchAll(HARDCODED_URL_RE)) {
		push(m, "hardcoded-url", m[2], stripQueryAndFragment(m[2]), false);
	}

	for (const m of content.matchAll(MARKDOWN_LINK_RE)) {
		push(m, "markdown-link", m[1], stripQueryAndFragment(m[1]), false);
	}

	return refs;
}

// ---------------------------------------------------------------------------
// File discovery.

const SKIP_DIRS = new Set([
	"node_modules",
	"dist",
	"build",
	".next",
	".cache",
	"out",
	".audit",
	".style",
	"storybook-static",
	"__generated__",
]);

export function walk(dir, exts, results = []) {
	let entries;
	try {
		entries = fs.readdirSync(dir, { withFileTypes: true });
	} catch (e) {
		if (e.code === "ENOENT" || e.code === "ENOTDIR") return results;
		throw e;
	}
	for (const e of entries) {
		const full = path.join(dir, e.name);
		if (e.isDirectory()) {
			if (SKIP_DIRS.has(e.name)) continue;
			walk(full, exts, results);
		} else if (e.isFile() && exts.some((ext) => e.name.endsWith(ext))) {
			results.push(full);
		}
	}
	return results;
}

// ---------------------------------------------------------------------------
// Report rendering.

export function repoForFile(file) {
	if (file.includes("/coder/site/")) return "coder/coder/site";
	if (file.includes("/coder.com/src/")) return "coder/coder.com/src";
	return "unknown";
}

export function relForFile(file) {
	// Strip absolute prefix so the report is portable.
	return file
		.replace(/^.*\/coder\/site\//, "site/")
		.replace(/^.*\/coder\.com\/src\//, "src/");
}

export function escapeMd(s) {
	return s.replace(/\|/g, "\\|");
}

export function suggestedFixForKind(kind, suggestedDestination) {
	// Reverse the /docs prefix transformation for docs() callsites so the
	// suggested fix is the literal string the developer should paste in.
	if (kind === "hardcoded-url") {
		return "https://coder.com" + suggestedDestination;
	}
	if (kind === "markdown-link") {
		return suggestedDestination;
	}
	// docs-literal / docs-template: helper prepends /docs, so strip it.
	return suggestedDestination.startsWith("/docs")
		? suggestedDestination.slice("/docs".length)
		: suggestedDestination;
}

export function buildReport({ findings, redirectsPath, roots, startedAt }) {
	const byRepo = Map.groupBy(findings, (f) => repoForFile(f.file));

	// Stable sort: dynamic last; otherwise by file then line.
	for (const list of byRepo.values()) {
		list.sort((a, b) => {
			if (a.dynamic !== b.dynamic) return a.dynamic ? 1 : -1;
			if (a.file !== b.file) return a.file < b.file ? -1 : 1;
			return a.lineNo - b.lineNo;
		});
	}

	const sectionRows = (list) =>
		list
			.map((f) => {
				const rel = relForFile(f.file);
				const fix = suggestedFixForKind(f.kind, f.match.suggestedDestination);
				const fragmentNote = f.rawArg.includes("#")
					? " (preserve `#anchor` from current value)"
					: "";
				return `| \`${rel}:${f.lineNo}\` | \`${escapeMd(f.rawArg)}\` | \`${escapeMd(f.match.redirect.source)}\` -> \`${escapeMd(f.match.redirect.destination)}\` | \`${escapeMd(fix)}\`${fragmentNote} | ${f.dynamic ? "Yes" : "No"} |`;
			})
			.join("\n");

	const totalDynamic = findings.filter((f) => f.dynamic).length;
	const totalStatic = findings.length - totalDynamic;

	const out = [
		"# Redirects audit: TS/TSX docs-URL references",
		"",
		`Generated: ${startedAt.toISOString()}`,
		"",
		"## Method",
		"",
		'This audit cross-references every static `docs("...")` call, every `docs(`...`)` template literal with a literal prefix, every hardcoded `coder.com/docs/...` URL, and every `](/docs/...)`-style Markdown link in TS/TSX files against the source side of every `/docs/*` rule in `coder/coder.com/redirects.json`. Anything that matches a redirect source is stale and needs to be updated to the destination.',
		"",
		`Source of truth for the redirect set: \`${redirectsPath}\` at audit time.`,
		"",
		"Scanned roots:",
		"",
		...roots.map((r) => `* \`${r}\``),
		"",
		"Pattern matchers:",
		"",
		'* `docs("/...")` and `docs(\'/...\')` and `docs(`/...`)` (no `${}`).',
		"* `docs(`/.../${expr}/...`)` (literal prefix only; flagged as dynamic for manual review).",
		"* Any string literal containing `https://coder.com/docs/...` or `https://*.coder.com/docs/...`.",
		"* Markdown-link form `](https://coder.com/docs/...)` or `](/docs/...)` inside prose, notifications, and mock data.",
		"",
		"Hash fragments (`#anchor`) and query strings (`?foo`) are stripped before redirect matching.",
		"",
		"## Summary",
		"",
		"| Total findings | Auto-fixable (literal) | Manual review (dynamic) |",
		"|---|---|---|",
		`| ${findings.length} | ${totalStatic} | ${totalDynamic} |`,
		"",
	];

	const repoOrder = ["coder/coder/site", "coder/coder.com/src"];
	for (const repo of repoOrder) {
		const list = byRepo.get(repo) ?? [];
		out.push(`## ${repo}`);
		out.push("");
		if (list.length === 0) {
			out.push("No findings.");
			out.push("");
			continue;
		}
		out.push(`${list.length} ${list.length === 1 ? "finding" : "findings"}.`);
		out.push("");
		out.push(
			"| File:Line | Current path | Redirect rule | Suggested fix | Dynamic? |",
		);
		out.push("|---|---|---|---|---|");
		out.push(sectionRows(list));
		out.push("");
	}

	const unknown = byRepo.get("unknown") ?? [];
	if (unknown.length > 0) {
		out.push("## Unclassified findings");
		out.push("");
		out.push(
			"These came from a path that did not match the known repo prefixes. Investigate.",
		);
		out.push("");
		out.push(
			"| File:Line | Current path | Redirect rule | Suggested fix | Dynamic? |",
		);
		out.push("|---|---|---|---|---|");
		out.push(sectionRows(unknown));
		out.push("");
	}

	out.push("## Notes");
	out.push("");
	out.push(
		"* Dynamic findings have a `${...}` expression somewhere in the path. The suggested fix shows what the literal prefix should become; the developer must keep the dynamic suffix intact.",
	);
	out.push(
		"* Findings under `docs/.audit/` are excluded by file discovery (`.audit` is in SKIP_DIRS) to avoid feedback loops on the audit itself.",
	);
	out.push(
		"* Re-run with `node site/scripts/audit-docs-paths.mjs` from the repo root.",
	);
	out.push("");

	return out.join("\n");
}

// ---------------------------------------------------------------------------
// CLI entry point.

function defaultOutForToday() {
	const today = new Date().toISOString().slice(0, 10);
	return `/home/coder/coder/docs/.audit/redirects-audit-${today}.md`;
}

export function runCli(argv) {
	const { values: args } = parseArgs({
		args: argv,
		options: {
			redirects: { type: "string" },
			roots: { type: "string" },
			out: { type: "string" },
		},
	});
	const redirectsPath = args.redirects ?? "/home/coder/coder.com/redirects.json";
	const roots = (
		args.roots ?? "/home/coder/coder/site/src,/home/coder/coder.com/src"
	)
		.split(",")
		.map((s) => s.trim())
		.filter(Boolean);
	const outPath = args.out ?? defaultOutForToday();

	const startedAt = new Date();
	console.error(`Loading redirects from ${redirectsPath}`);
	const redirects = docsRedirects(loadRedirects(redirectsPath));
	console.error(`  ${redirects.length} /docs/* redirect rules indexed`);

	const exts = [".ts", ".tsx"];
	const allFiles = [];
	for (const root of roots) {
		if (!fs.existsSync(root)) {
			console.error(`  WARNING: ${root} does not exist; skipping`);
			continue;
		}
		console.error(`Scanning ${root}`);
		const found = walk(root, exts);
		console.error(`  ${found.length} files`);
		allFiles.push(...found);
	}

	const findings = [];
	for (const file of allFiles) {
		const content = fs.readFileSync(file, "utf-8");
		const refs = extractReferences(file, content);
		for (const ref of refs) {
			const match = findMatchingRedirect(ref.docsPath, redirects);
			if (match) findings.push({ ...ref, match });
		}
	}

	console.error(`Total findings: ${findings.length}`);

	const report = buildReport({
		findings,
		redirectsPath,
		roots,
		startedAt,
	});

	fs.mkdirSync(path.dirname(outPath), { recursive: true });
	fs.writeFileSync(outPath, report);
	const totalDynamic = findings.filter((f) => f.dynamic).length;
	const totalStatic = findings.length - totalDynamic;
	console.error(`Wrote ${outPath}`);
	console.error(`  Static: ${totalStatic}`);
	console.error(`  Dynamic: ${totalDynamic}`);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
	runCli(process.argv.slice(2));
}
