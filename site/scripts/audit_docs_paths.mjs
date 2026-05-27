#!/usr/bin/env node
// Audit script for DOCS-253 (parent: DOCS-209).
//
// Cross-references docs(...) and string-literal docs-URL references in TS/TSX
// files against the source side of every /docs/* rule in
// coder.com/redirects.json. Anything that matches a redirect source is stale
// and needs to be updated to the redirect's destination.
//
// Usage:
//   node site/scripts/audit_docs_paths.mjs \
//     --redirects=/path/to/coder.com/redirects.json \
//     --roots=/path/to/coder/site,/path/to/coder.com/src \
//     --out=docs/.audit/redirects-audit-YYYY-MM-DD.md
//
// All flags are optional. Defaults assume a standard Coder dev layout under
// /home/coder/. The script never modifies source files; it only emits the
// report.

import fs from "node:fs";
import path from "node:path";

const args = Object.fromEntries(
	process.argv
		.slice(2)
		.map((arg) => {
			const m = arg.match(/^--([^=]+)=(.*)$/);
			return m ? [m[1], m[2]] : null;
		})
		.filter(Boolean),
);

const REDIRECTS = args.redirects ?? "/home/coder/coder.com/redirects.json";
const ROOTS = (args.roots ?? "/home/coder/coder/site/src,/home/coder/coder.com/src")
	.split(",")
	.map((s) => s.trim())
	.filter(Boolean);
const OUT = args.out ?? "/home/coder/coder/docs/.audit/redirects-audit-2026-05-27.md";

// ---------------------------------------------------------------------------
// Load and index redirects.

function loadRedirects(p) {
	const text = fs.readFileSync(p, "utf-8");
	return JSON.parse(text);
}

// Build an ordered list of /docs/* redirect rules. Order matters because
// Next.js picks the first matching rule.
function docsRedirects(all) {
	return all.filter((r) => typeof r.source === "string" && r.source.startsWith("/docs/"));
}

// Match a path against a single redirect source. Returns the redirect's
// destination with path params substituted, or null if no match.
function matchRedirect(refPath, redirect) {
	const src = redirect.source;
	const dst = redirect.destination;

	// Exact match.
	if (src === refPath) return dst;

	// Trailing /:path* wildcard (Next.js's "match anything below this prefix").
	if (src.endsWith("/:path*")) {
		const prefix = src.slice(0, -"/:path*".length);
		if (refPath === prefix) {
			// Empty :path*. Strip the trailing /:path* from destination.
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

	// Anything else (e.g. :version(...) capture groups) is rare in /docs/* rules
	// and not used for the audit. Skip.
	return null;
}

function findMatchingRedirect(refPath, redirects) {
	for (const r of redirects) {
		const dst = matchRedirect(refPath, r);
		if (dst !== null) return { redirect: r, suggestedDestination: dst };
	}
	return null;
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

function walk(dir, exts, results = []) {
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
// Reference extraction.

// docs("/path") | docs('/path') | docs(`/path`) with NO ${expr}.
// Allows whitespace and optional trailing args (rare).
const DOCS_LITERAL_RE = /\bdocs\(\s*(['"`])([^'"`)$]+)\1/g;

// docs(`/path/${expr}/more`). We capture the whole literal segment between the
// backticks so we can flag the literal prefix.
const DOCS_TEMPLATE_RE = /\bdocs\(\s*`([^`]*\$\{[^`]*\}[^`]*)`/g;

// "https://coder.com/docs/..." inside any string literal.
const HARDCODED_URL_RE = /['"`]https?:\/\/(?:[a-z0-9-]+\.)?coder\.com(\/docs\/[^'"`)\s]+)['"`]/g;

function extractReferences(filePath, content) {
	const refs = [];
	const lineStarts = [0];
	for (let i = 0; i < content.length; i++) {
		if (content[i] === "\n") lineStarts.push(i + 1);
	}
	const indexToLine = (idx) => {
		// Binary search for the largest lineStart <= idx.
		let lo = 0,
			hi = lineStarts.length - 1;
		while (lo < hi) {
			const mid = (lo + hi + 1) >>> 1;
			if (lineStarts[mid] <= idx) lo = mid;
			else hi = mid - 1;
		}
		return lo + 1; // 1-based
	};
	const lineAt = (idx) => {
		const lineNo = indexToLine(idx);
		const start = lineStarts[lineNo - 1];
		const end = lineStarts[lineNo] ?? content.length;
		return content.slice(start, end).replace(/\n$/, "").trim();
	};

	let m;
	DOCS_LITERAL_RE.lastIndex = 0;
	while ((m = DOCS_LITERAL_RE.exec(content)) !== null) {
		refs.push({
			file: filePath,
			lineNo: indexToLine(m.index),
			snippet: lineAt(m.index),
			kind: "docs-literal",
			rawArg: m[2],
			docsPath: "/docs" + stripFragments(m[2]),
			dynamic: false,
		});
	}

	DOCS_TEMPLATE_RE.lastIndex = 0;
	while ((m = DOCS_TEMPLATE_RE.exec(content)) !== null) {
		refs.push({
			file: filePath,
			lineNo: indexToLine(m.index),
			snippet: lineAt(m.index),
			kind: "docs-template",
			rawArg: m[1],
			docsPath: "/docs" + stripFragments(literalPrefix(m[1])),
			dynamic: true,
		});
	}

	HARDCODED_URL_RE.lastIndex = 0;
	while ((m = HARDCODED_URL_RE.exec(content)) !== null) {
		refs.push({
			file: filePath,
			lineNo: indexToLine(m.index),
			snippet: lineAt(m.index),
			kind: "hardcoded-url",
			rawArg: m[1],
			docsPath: stripFragments(m[1]),
			dynamic: false,
		});
	}
	return refs;
}

function stripFragments(p) {
	// Remove hash fragment and query string for redirect matching.
	let out = p;
	const hash = out.indexOf("#");
	if (hash !== -1) out = out.slice(0, hash);
	const query = out.indexOf("?");
	if (query !== -1) out = out.slice(0, query);
	return out;
}

function literalPrefix(tmpl) {
	// Return the literal portion before the first ${...} so we can do a partial
	// redirect match on the static prefix.
	const idx = tmpl.indexOf("${");
	return idx === -1 ? tmpl : tmpl.slice(0, idx);
}

// ---------------------------------------------------------------------------
// Main.

const startedAt = new Date();
console.error(`Loading redirects from ${REDIRECTS}`);
const redirects = docsRedirects(loadRedirects(REDIRECTS));
console.error(`  ${redirects.length} /docs/* redirect rules indexed`);

const exts = [".ts", ".tsx"];
const allFiles = [];
for (const root of ROOTS) {
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
		if (match) {
			findings.push({ ...ref, match });
		}
	}
}

console.error(`Total findings: ${findings.length}`);

// Group by repo for the report.
function repoForFile(file) {
	if (file.includes("/coder/site/")) return "coder/coder/site";
	if (file.includes("/coder.com/src/")) return "coder/coder.com/src";
	return "unknown";
}

function relForFile(file) {
	// Strip absolute prefix so the report is portable.
	return file
		.replace(/^.*\/coder\/site\//, "site/")
		.replace(/^.*\/coder\.com\/src\//, "src/");
}

const byRepo = new Map();
for (const f of findings) {
	const repo = repoForFile(f.file);
	if (!byRepo.has(repo)) byRepo.set(repo, []);
	byRepo.get(repo).push(f);
}

// Stable sort: dynamic last; otherwise by file then line.
for (const list of byRepo.values()) {
	list.sort((a, b) => {
		if (a.dynamic !== b.dynamic) return a.dynamic ? 1 : -1;
		if (a.file !== b.file) return a.file < b.file ? -1 : 1;
		return a.lineNo - b.lineNo;
	});
}

// ---------------------------------------------------------------------------
// Report.

const sectionRows = (list) =>
	list
		.map((f) => {
			const rel = relForFile(f.file);
			const fixDocsPath = f.match.suggestedDestination;
			// Reverse the /docs prefix transformation for docs() callsites so the
			// suggested fix is the literal string the developer should paste in.
			const fix =
				f.kind === "hardcoded-url"
					? "https://coder.com" + fixDocsPath
					: fixDocsPath.startsWith("/docs")
					? fixDocsPath.slice("/docs".length)
					: fixDocsPath;
			const fragmentNote = f.rawArg.includes("#") ? " (preserve `#anchor` from current value)" : "";
			return `| \`${rel}:${f.lineNo}\` | \`${escapeMd(f.rawArg)}\` | \`${escapeMd(f.match.redirect.source)}\` -> \`${escapeMd(f.match.redirect.destination)}\` | \`${escapeMd(fix)}\`${fragmentNote} | ${f.dynamic ? "Yes" : "No"} |`;
		})
		.join("\n");

function escapeMd(s) {
	return s.replace(/\|/g, "\\|");
}

const totalDynamic = findings.filter((f) => f.dynamic).length;
const totalStatic = findings.length - totalDynamic;

const report = [
	`# Redirects audit: TS/TSX docs-URL references`,
	``,
	`Generated: ${startedAt.toISOString()}`,
	``,
	`Tracks: [DOCS-253](https://linear.app/codercom/issue/DOCS-253) (parent: [DOCS-209](https://linear.app/codercom/issue/DOCS-209)).`,
	``,
	`## Method`,
	``,
	`This audit cross-references every static \`docs("...")\` call, every \`docs(\\\`...\\\`)\` template literal with a literal prefix, and every hardcoded \`coder.com/docs/...\` URL in TS/TSX files against the source side of every \`/docs/*\` rule in \`coder/coder.com/redirects.json\`. Anything that matches a redirect source is stale and needs to be updated to the destination.`,
	``,
	`Source of truth for the redirect set: \`${REDIRECTS}\` at audit time.`,
	``,
	`Scanned roots:`,
	``,
	...ROOTS.map((r) => `* \`${r}\``),
	``,
	`Pattern matchers:`,
	``,
	`* \`docs("/...")\` and \`docs('/...')\` and \`docs(\\\`/...\\\`)\` (no \`\${}\`).`,
	`* \`docs(\\\`/.../$\\{expr\\}/...\\\`)\` (literal prefix only; flagged as dynamic for manual review).`,
	`* Any string literal containing \`https://coder.com/docs/...\` or \`https://*.coder.com/docs/...\`.`,
	``,
	`Hash fragments (\`#anchor\`) and query strings (\`?foo\`) are stripped before redirect matching.`,
	``,
	`## Summary`,
	``,
	`| Total findings | Auto-fixable (literal) | Manual review (dynamic) |`,
	`|---|---|---|`,
	`| ${findings.length} | ${totalStatic} | ${totalDynamic} |`,
	``,
];

const repoOrder = ["coder/coder/site", "coder/coder.com/src"];
for (const repo of repoOrder) {
	const list = byRepo.get(repo) ?? [];
	report.push(`## ${repo}`);
	report.push(``);
	if (list.length === 0) {
		report.push(`No findings.`);
		report.push(``);
		continue;
	}
	report.push(`${list.length} findings.`);
	report.push(``);
	report.push(`| File:Line | Current path | Redirect rule | Suggested fix | Dynamic? |`);
	report.push(`|---|---|---|---|---|`);
	report.push(sectionRows(list));
	report.push(``);
}

// Any remaining repos under "unknown".
const unknown = byRepo.get("unknown") ?? [];
if (unknown.length > 0) {
	report.push(`## Unclassified findings`);
	report.push(``);
	report.push(`These came from a path that did not match the known repo prefixes. Investigate.`);
	report.push(``);
	report.push(`| File:Line | Current path | Redirect rule | Suggested fix | Dynamic? |`);
	report.push(`|---|---|---|---|---|`);
	report.push(sectionRows(unknown));
	report.push(``);
}

report.push(`## Notes`);
report.push(``);
report.push(`* Dynamic findings have a \`\${...}\` expression somewhere in the path. The suggested fix shows what the literal prefix should become; the developer must keep the dynamic suffix intact.`);
report.push(`* Findings under \`docs/.audit/\` or \`docs/CHANGELOG\` paths are excluded by file discovery to avoid feedback loops on the audit itself.`);
report.push(`* Re-run with \`node site/scripts/audit_docs_paths.mjs\` from the repo root.`);
report.push(``);

const outDir = path.dirname(OUT);
fs.mkdirSync(outDir, { recursive: true });
fs.writeFileSync(OUT, report.join("\n"));
console.error(`Wrote ${OUT}`);
console.error(`  Static: ${totalStatic}`);
console.error(`  Dynamic: ${totalDynamic}`);
