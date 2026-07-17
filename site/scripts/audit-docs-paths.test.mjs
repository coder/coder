import { describe, expect, it, vi } from "vitest";
import {
	DOCS_LITERAL_RE,
	DOCS_TEMPLATE_RE,
	HARDCODED_URL_RE,
	MARKDOWN_LINK_RE,
	buildReport,
	docsRedirects,
	escapeMd,
	extractReferences,
	findMatchingRedirect,
	literalPrefix,
	matchRedirect,
	relForFile,
	repoForFile,
	runCli,
	stripQueryAndFragment,
	suggestedFixForKind,
	walk,
} from "./audit-docs-paths.mjs";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const rule = (source, destination) => ({ source, destination, permanent: true });

describe("docsRedirects", () => {
	it("keeps only /docs/* sources", () => {
		const all = [
			rule("/docs/a", "/docs/b"),
			rule("/api/old", "/api/new"),
			rule("/docs/c/:path*", "/docs/d/:path*"),
			{ source: 42, destination: "/" }, // malformed; must be skipped
		];
		expect(docsRedirects(all)).toEqual([
			rule("/docs/a", "/docs/b"),
			rule("/docs/c/:path*", "/docs/d/:path*"),
		]);
	});

	it("throws a TypeError when given a non-array (e.g. object)", () => {
		expect(() => docsRedirects({})).toThrowError(TypeError);
		expect(() => docsRedirects({})).toThrowError(/got object/);
	});

	it("throws a TypeError when given a non-array (e.g. string)", () => {
		expect(() => docsRedirects("hello")).toThrowError(TypeError);
		expect(() => docsRedirects("hello")).toThrowError(/got string/);
	});

	it("throws a TypeError when given null", () => {
		expect(() => docsRedirects(null)).toThrowError(TypeError);
	});
});

describe("matchRedirect", () => {
	it("returns destination on an exact match", () => {
		expect(
			matchRedirect("/docs/admin/rbac", rule("/docs/admin/rbac", "/docs/x")),
		).toBe("/docs/x");
	});

	it("returns null when nothing matches", () => {
		expect(matchRedirect("/docs/foo", rule("/docs/bar", "/docs/baz"))).toBe(null);
	});

	it("matches /:path* with a real subpath and substitutes the tail", () => {
		expect(
			matchRedirect(
				"/docs/old/sub/page",
				rule("/docs/old/:path*", "/docs/new/:path*"),
			),
		).toBe("/docs/new/sub/page");
	});

	it("matches /:path* with an empty path (bare prefix)", () => {
		expect(
			matchRedirect("/docs/old", rule("/docs/old/:path*", "/docs/new/:path*")),
		).toBe("/docs/new");
	});

	it("matches /:path* when the destination drops the wildcard", () => {
		expect(
			matchRedirect("/docs/old/x/y", rule("/docs/old/:path*", "/docs/new")),
		).toBe("/docs/new");
	});

	it("matches :slug(.*) and substitutes the tail", () => {
		expect(
			matchRedirect(
				"/docs/platforms/kubernetes",
				rule("/docs/platforms/:slug(.*)", "/docs/install/:slug"),
			),
		).toBe("/docs/install/kubernetes");
	});

	it("matches :slug(.*) when destination drops the slug", () => {
		expect(
			matchRedirect(
				"/docs/platforms/aws",
				rule("/docs/platforms/:slug(.*)", "/docs/install/cloud"),
			),
		).toBe("/docs/install/cloud");
	});

	it("does not partial-match a path that overshoots the prefix", () => {
		expect(
			matchRedirect(
				"/docs/oldsibling",
				rule("/docs/old/:path*", "/docs/new/:path*"),
			),
		).toBe(null);
	});
});

describe("findMatchingRedirect", () => {
	const redirects = [
		rule("/docs/admin/rbac", "/docs/admin/templates/template-permissions"),
		rule("/docs/old/:path*", "/docs/new/:path*"),
	];

	it("returns the first matching rule and its destination", () => {
		const got = findMatchingRedirect("/docs/admin/rbac", redirects);
		expect(got).not.toBeNull();
		expect(got.redirect).toBe(redirects[0]);
		expect(got.suggestedDestination).toBe(
			"/docs/admin/templates/template-permissions",
		);
	});

	it("returns null when no rule matches", () => {
		expect(findMatchingRedirect("/docs/never", redirects)).toBeNull();
	});
});

describe("stripQueryAndFragment", () => {
	it("removes a hash fragment", () => {
		expect(stripQueryAndFragment("/docs/a#section")).toBe("/docs/a");
	});

	it("removes a query string", () => {
		expect(stripQueryAndFragment("/docs/a?x=1")).toBe("/docs/a");
	});

	it("removes both when query precedes hash", () => {
		expect(stripQueryAndFragment("/docs/a?x=1#section")).toBe("/docs/a");
	});

	it("removes both when hash precedes query", () => {
		// Slices at the first hash; query suffix after the hash is dropped too.
		expect(stripQueryAndFragment("/docs/a#section?x=1")).toBe("/docs/a");
	});

	it("is a no-op when neither is present", () => {
		expect(stripQueryAndFragment("/docs/a/b")).toBe("/docs/a/b");
	});
});

describe("literalPrefix", () => {
	it("returns the whole string when there is no interpolation", () => {
		expect(literalPrefix("/docs/a/b")).toBe("/docs/a/b");
	});

	it("returns the prefix up to the first ${...}", () => {
		expect(literalPrefix("/docs/a/${slug}/b")).toBe("/docs/a/");
	});

	it("handles interpolation at position zero", () => {
		expect(literalPrefix("${root}/docs/a")).toBe("");
	});
});

describe("regex patterns", () => {
	const exec = (re, content) => [...content.matchAll(re)].map((m) => [...m]);

	it("DOCS_LITERAL_RE matches docs(\"...\"), docs('...'), docs(`...`)", () => {
		const src = `docs("/a/b") docs('/c/d') docs(\`/e/f\`)`;
		const m = exec(DOCS_LITERAL_RE, src);
		expect(m.map((row) => row[2])).toEqual(["/a/b", "/c/d", "/e/f"]);
	});

	it("DOCS_LITERAL_RE skips template literals with interpolation", () => {
		const src = "docs(`/a/${x}/b`)";
		expect(exec(DOCS_LITERAL_RE, src)).toHaveLength(0);
	});

	it("DOCS_TEMPLATE_RE matches docs(`/.../${expr}/...`)", () => {
		const src = "docs(`/a/${x}/b`)";
		const m = exec(DOCS_TEMPLATE_RE, src);
		expect(m).toHaveLength(1);
		expect(m[0][1]).toBe("/a/${x}/b");
	});

	it("HARDCODED_URL_RE matches https://coder.com URLs in any quote", () => {
		const src = `a = "https://coder.com/docs/foo"; b = 'https://coder.com/docs/bar';`;
		const m = exec(HARDCODED_URL_RE, src);
		expect(m.map((row) => row[2])).toEqual(["/docs/foo", "/docs/bar"]);
	});

	it("HARDCODED_URL_RE matches *.coder.com subdomains", () => {
		const src = `"https://dev.coder.com/docs/foo"`;
		const m = exec(HARDCODED_URL_RE, src);
		expect(m).toHaveLength(1);
		expect(m[0][2]).toBe("/docs/foo");
	});

	it("HARDCODED_URL_RE does NOT match markdown-link form", () => {
		// The URL is bounded by ( and ), not by quotes. This pattern misses
		// the URL; MARKDOWN_LINK_RE catches it.
		const src = `[label](https://coder.com/docs/foo)`;
		expect(exec(HARDCODED_URL_RE, src)).toHaveLength(0);
	});

	it("HARDCODED_URL_RE rejects mismatched opening and closing delimiters", () => {
		// Backreference ensures "...' or '...\" cannot match.
		const src = `a = "https://coder.com/docs/foo'; b = 'https://coder.com/docs/bar";`;
		expect(exec(HARDCODED_URL_RE, src)).toHaveLength(0);
	});

	it("MARKDOWN_LINK_RE matches full URLs in markdown links", () => {
		const src = `[label](https://coder.com/docs/foo) and [x](https://dev.coder.com/docs/bar)`;
		const m = exec(MARKDOWN_LINK_RE, src);
		expect(m.map((row) => row[1])).toEqual(["/docs/foo", "/docs/bar"]);
	});

	it("MARKDOWN_LINK_RE matches relative /docs/... links too", () => {
		const src = `See [the docs](/docs/admin/rbac) for details.`;
		const m = exec(MARKDOWN_LINK_RE, src);
		expect(m.map((row) => row[1])).toEqual(["/docs/admin/rbac"]);
	});

	it("MARKDOWN_LINK_RE ignores trailing punctuation outside the parentheses", () => {
		const src = `See [the docs](/docs/x).`;
		const m = exec(MARKDOWN_LINK_RE, src);
		expect(m[0][1]).toBe("/docs/x");
	});
});

describe("extractReferences", () => {
	it("captures all four kinds and sets line numbers (1-based)", () => {
		const content = [
			'docs("/admin/rbac");', // line 1
			"const x = `${base}/docs/admin/groups`;", // line 2: no match
			'const y = "https://coder.com/docs/admin/quotas";', // line 3
			"// see [the docs](/docs/admin/audit-logs)", // line 4
			"docs(`/templates/${slug}/edit`);", // line 5
		].join("\n");
		const refs = extractReferences("/tmp/foo.ts", content);
		const byKind = (k) => refs.filter((r) => r.kind === k);

		expect(byKind("docs-literal")).toEqual([
			expect.objectContaining({
				lineNo: 1,
				rawArg: "/admin/rbac",
				docsPath: "/docs/admin/rbac",
				dynamic: false,
			}),
		]);

		expect(byKind("hardcoded-url")).toEqual([
			expect.objectContaining({
				lineNo: 3,
				rawArg: "/docs/admin/quotas",
				docsPath: "/docs/admin/quotas",
				dynamic: false,
			}),
		]);

		expect(byKind("markdown-link")).toEqual([
			expect.objectContaining({
				lineNo: 4,
				rawArg: "/docs/admin/audit-logs",
				docsPath: "/docs/admin/audit-logs",
				dynamic: false,
			}),
		]);

		expect(byKind("docs-template")).toEqual([
			expect.objectContaining({
				lineNo: 5,
				docsPath: "/docs/templates/",
				dynamic: true,
			}),
		]);
	});

	it("strips fragments from the redirect-matching docsPath but preserves rawArg", () => {
		const content = `docs("/admin/rbac#perms");`;
		const refs = extractReferences("/tmp/foo.ts", content);
		expect(refs[0].rawArg).toBe("/admin/rbac#perms");
		expect(refs[0].docsPath).toBe("/docs/admin/rbac");
	});

	it("returns an empty array when content has no docs refs", () => {
		expect(extractReferences("/tmp/foo.ts", "const x = 1;")).toEqual([]);
	});

	it("handles a multi-line docs() invocation", () => {
		const content = [
			"link={docs(",
			'  "/admin/rbac",',
			")}",
		].join("\n");
		const refs = extractReferences("/tmp/foo.tsx", content);
		expect(refs).toHaveLength(1);
		expect(refs[0].kind).toBe("docs-literal");
		expect(refs[0].docsPath).toBe("/docs/admin/rbac");
		// The match starts on the line with "docs(", which is line 1.
		expect(refs[0].lineNo).toBe(1);
	});
});

describe("repoForFile / relForFile", () => {
	it("classifies coder/coder/site/ files", () => {
		expect(repoForFile("/home/coder/coder/site/src/app.tsx")).toBe(
			"coder/coder/site",
		);
	});

	it("classifies coder/coder.com/src/ files", () => {
		expect(repoForFile("/home/coder/coder.com/src/foo.ts")).toBe(
			"coder/coder.com/src",
		);
	});

	it("returns 'unknown' for paths that do not match a known repo", () => {
		expect(repoForFile("/var/tmp/foo.ts")).toBe("unknown");
	});

	it("rewrites coder/coder/site paths to a site/ relative form", () => {
		expect(relForFile("/home/coder/coder/site/src/app.tsx")).toBe(
			"site/src/app.tsx",
		);
	});

	it("rewrites coder.com paths to an src/ relative form", () => {
		expect(relForFile("/home/coder/coder.com/src/data/x.ts")).toBe(
			"src/data/x.ts",
		);
	});
});

describe("escapeMd", () => {
	it("escapes pipes so table rows do not break", () => {
		expect(escapeMd("a|b|c")).toBe("a\\|b\\|c");
	});

	it("leaves other characters alone", () => {
		expect(escapeMd("/docs/a-b.c")).toBe("/docs/a-b.c");
	});
});

describe("suggestedFixForKind", () => {
	it("strips /docs from docs-literal suggestions", () => {
		expect(suggestedFixForKind("docs-literal", "/docs/admin/x")).toBe(
			"/admin/x",
		);
	});

	it("strips /docs from docs-template suggestions", () => {
		expect(suggestedFixForKind("docs-template", "/docs/admin/x")).toBe(
			"/admin/x",
		);
	});

	it("prefixes hardcoded-url suggestions with https://coder.com", () => {
		expect(suggestedFixForKind("hardcoded-url", "/docs/admin/x")).toBe(
			"https://coder.com/docs/admin/x",
		);
	});

	it("returns markdown-link suggestions verbatim", () => {
		expect(suggestedFixForKind("markdown-link", "/docs/admin/x")).toBe(
			"/docs/admin/x",
		);
	});
});

describe("buildReport", () => {
	const finding = (file, lineNo, overrides = {}) => ({
		file,
		lineNo,
		kind: "docs-literal",
		rawArg: "/admin/rbac",
		docsPath: "/docs/admin/rbac",
		dynamic: false,
		match: {
			redirect: rule(
				"/docs/admin/rbac",
				"/docs/admin/templates/template-permissions",
			),
			suggestedDestination: "/docs/admin/templates/template-permissions",
		},
		...overrides,
	});

	const startedAt = new Date("2026-05-27T12:00:00.000Z");

	it("renders an empty report with zero findings", () => {
		const report = buildReport({
			findings: [],
			redirectsPath: "/redirects.json",
			roots: ["/home/coder/coder/site/src"],
			startedAt,
		});
		expect(report).toContain("# Redirects audit");
		expect(report).toContain("| 0 | 0 | 0 |");
		expect(report).toContain("## coder/coder/site");
		expect(report).toContain("No findings.");
		expect(report).toContain("/redirects.json");
		expect(report).toContain("`/home/coder/coder/site/src`");
	});

	it("groups findings by repo, counts dynamic/static, and sorts within sections", () => {
		const findings = [
			// Dynamic finding under site (sorted last in section).
			finding("/home/coder/coder/site/src/b.tsx", 5, {
				kind: "docs-template",
				rawArg: "/templates/${slug}/edit",
				docsPath: "/docs/templates/",
				dynamic: true,
			}),
			// Static finding under site, file b, later line.
			finding("/home/coder/coder/site/src/b.tsx", 20),
			// Static finding under site, file a (sorts before file b).
			finding("/home/coder/coder/site/src/a.tsx", 10),
			// Static finding under coder.com/src.
			finding("/home/coder/coder.com/src/page.ts", 3),
		];
		const report = buildReport({
			findings,
			redirectsPath: "/redirects.json",
			roots: ["/home/coder/coder/site/src", "/home/coder/coder.com/src"],
			startedAt,
		});

		// Summary: 4 total, 3 static, 1 dynamic.
		expect(report).toContain("| 4 | 3 | 1 |");

		// Both repo sections present, each with finding counts.
		expect(report).toMatch(/## coder\/coder\/site\n\n3 findings\./);
		expect(report).toMatch(/## coder\/coder\.com\/src\n\n1 finding\./);

		// Within the site section, file a should appear before file b, and the
		// dynamic finding should appear after the static ones from the same file.
		const siteIdx = report.indexOf("## coder/coder/site");
		const comIdx = report.indexOf("## coder/coder.com/src");
		const siteSection = report.slice(siteIdx, comIdx);
		const aIdx = siteSection.indexOf("site/src/a.tsx:10");
		const bStaticIdx = siteSection.indexOf("site/src/b.tsx:20");
		const bDynamicIdx = siteSection.indexOf("site/src/b.tsx:5");
		expect(aIdx).toBeGreaterThan(-1);
		expect(bStaticIdx).toBeGreaterThan(-1);
		expect(bDynamicIdx).toBeGreaterThan(-1);
		expect(aIdx).toBeLessThan(bStaticIdx);
		expect(bStaticIdx).toBeLessThan(bDynamicIdx);

		// Dynamic? column reflects the dynamic flag.
		expect(siteSection).toMatch(/site\/src\/a\.tsx:10\b[^\n]*\| No \|/);
		expect(siteSection).toMatch(/site\/src\/b\.tsx:5\b[^\n]*\| Yes \|/);
	});

	it("annotates findings whose rawArg contains a # fragment", () => {
		const report = buildReport({
			findings: [
				finding("/home/coder/coder/site/src/a.tsx", 1, {
					rawArg: "/admin/rbac#perms",
				}),
			],
			redirectsPath: "/redirects.json",
			roots: ["/home/coder/coder/site/src"],
			startedAt,
		});
		expect(report).toContain("(preserve `#anchor` from current value)");
	});

	it("emits an Unclassified findings section for paths outside known repos", () => {
		const report = buildReport({
			findings: [finding("/var/tmp/foo.ts", 1)],
			redirectsPath: "/redirects.json",
			roots: ["/var/tmp"],
			startedAt,
		});
		expect(report).toContain("## Unclassified findings");
		expect(report).toContain("/var/tmp/foo.ts:1");
	});
});

describe("walk", () => {
	// Builds a temp tree of the shape:
	//   <tmpDir>/a.ts
	//   <tmpDir>/b.tsx
	//   <tmpDir>/c.js
	//   <tmpDir>/nested/d.ts
	//   <tmpDir>/nested/e.md
	//   <tmpDir>/node_modules/skipme.ts
	//   <tmpDir>/dist/also-skipped.ts
	//   <tmpDir>/.audit/audit-report.ts
	const buildTree = (root) => {
		fs.writeFileSync(path.join(root, "a.ts"), "export const a = 1;");
		fs.writeFileSync(path.join(root, "b.tsx"), "export const B = () => null;");
		fs.writeFileSync(path.join(root, "c.js"), "module.exports = {};");
		fs.mkdirSync(path.join(root, "nested"));
		fs.writeFileSync(path.join(root, "nested", "d.ts"), "export const d = 1;");
		fs.writeFileSync(path.join(root, "nested", "e.md"), "# notes");
		fs.mkdirSync(path.join(root, "node_modules"));
		fs.writeFileSync(
			path.join(root, "node_modules", "skipme.ts"),
			"export const x = 1;",
		);
		fs.mkdirSync(path.join(root, "dist"));
		fs.writeFileSync(
			path.join(root, "dist", "also-skipped.ts"),
			"export const x = 1;",
		);
		fs.mkdirSync(path.join(root, ".audit"));
		fs.writeFileSync(
			path.join(root, ".audit", "audit-report.ts"),
			"export const x = 1;",
		);
	};

	it("returns matching files, recurses, and filters by extension", () => {
		const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "audit-docs-walk-"));
		try {
			buildTree(tmpDir);
			const found = walk(tmpDir, [".ts", ".tsx"]).sort();
			const rel = found.map((f) => path.relative(tmpDir, f)).sort();
			expect(rel).toEqual([
				"a.ts",
				"b.tsx",
				path.join("nested", "d.ts"),
			]);
			// c.js excluded by extension filter; e.md excluded by extension filter;
			// node_modules/dist/.audit excluded by SKIP_DIRS.
			expect(rel).not.toContain("c.js");
			expect(rel).not.toContain(path.join("nested", "e.md"));
			expect(rel).not.toContain(path.join("node_modules", "skipme.ts"));
			expect(rel).not.toContain(path.join("dist", "also-skipped.ts"));
			expect(rel).not.toContain(path.join(".audit", "audit-report.ts"));
		} finally {
			fs.rmSync(tmpDir, { recursive: true, force: true });
		}
	});

	it("honors the extension filter (.tsx only)", () => {
		const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "audit-docs-walk-"));
		try {
			buildTree(tmpDir);
			const found = walk(tmpDir, [".tsx"]);
			const rel = found.map((f) => path.relative(tmpDir, f));
			expect(rel).toEqual(["b.tsx"]);
		} finally {
			fs.rmSync(tmpDir, { recursive: true, force: true });
		}
	});

	it("returns an empty array when the directory does not exist", () => {
		const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "audit-docs-walk-"));
		try {
			const missing = path.join(tmpDir, "does-not-exist");
			expect(walk(missing, [".ts"])).toEqual([]);
		} finally {
			fs.rmSync(tmpDir, { recursive: true, force: true });
		}
	});

	it("returns an empty array when handed a file instead of a directory", () => {
		const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "audit-docs-walk-"));
		try {
			const filePath = path.join(tmpDir, "a.ts");
			fs.writeFileSync(filePath, "export const a = 1;");
			expect(walk(filePath, [".ts"])).toEqual([]);
		} finally {
			fs.rmSync(tmpDir, { recursive: true, force: true });
		}
	});

	it("appends to an existing results array instead of replacing it", () => {
		const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "audit-docs-walk-"));
		try {
			fs.writeFileSync(path.join(tmpDir, "a.ts"), "export const a = 1;");
			const seed = ["/seed/value.ts"];
			const out = walk(tmpDir, [".ts"], seed);
			expect(out).toBe(seed);
			expect(out).toContain("/seed/value.ts");
			expect(out.some((f) => f.endsWith("a.ts"))).toBe(true);
		} finally {
			fs.rmSync(tmpDir, { recursive: true, force: true });
		}
	});
});

describe("runCli", () => {
	// Minimal redirects file written to a tmp dir so the CLI can load it.
	const writeTmpRedirects = (tmpDir) => {
		const redirectsPath = path.join(tmpDir, "redirects.json");
		fs.writeFileSync(
			redirectsPath,
			JSON.stringify([
				rule("/docs/admin/rbac", "/docs/admin/templates/template-permissions"),
			]),
		);
		return redirectsPath;
	};

	it("warns and continues when a --roots directory does not exist", () => {
		const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "audit-docs-cli-"));
		try {
			const redirectsPath = writeTmpRedirects(tmpDir);
			const missingRoot = path.join(tmpDir, "does-not-exist");
			const outPath = path.join(tmpDir, "out.md");

			const errSpy = vi.spyOn(console, "error").mockImplementation(() => {});
			let stderr;
			try {
				runCli([
					`--redirects=${redirectsPath}`,
					`--roots=${missingRoot}`,
					`--out=${outPath}`,
				]);
				stderr = errSpy.mock.calls.map((c) => c.join(" ")).join("\n");
			} finally {
				errSpy.mockRestore();
			}

			expect(stderr).toContain(missingRoot);
			expect(stderr).toContain("does not exist");

			// Report was still written with zero findings.
			expect(fs.existsSync(outPath)).toBe(true);
			const report = fs.readFileSync(outPath, "utf-8");
			expect(report).toContain("| 0 | 0 | 0 |");
		} finally {
			fs.rmSync(tmpDir, { recursive: true, force: true });
		}
	});

	it("writes a report when --roots contains a real directory with no matches", () => {
		const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "audit-docs-cli-"));
		try {
			const redirectsPath = writeTmpRedirects(tmpDir);
			const emptyRoot = path.join(tmpDir, "empty");
			fs.mkdirSync(emptyRoot);
			const outPath = path.join(tmpDir, "out.md");

			const errSpy = vi.spyOn(console, "error").mockImplementation(() => {});
			try {
				runCli([
					`--redirects=${redirectsPath}`,
					`--roots=${emptyRoot}`,
					`--out=${outPath}`,
				]);
			} finally {
				errSpy.mockRestore();
			}

			expect(fs.existsSync(outPath)).toBe(true);
			const report = fs.readFileSync(outPath, "utf-8");
			expect(report).toContain("| 0 | 0 | 0 |");
		} finally {
			fs.rmSync(tmpDir, { recursive: true, force: true });
		}
	});

	it("wires the full pipeline end-to-end and reports a real finding", () => {
		// Exercises walk -> readFile -> extractReferences ->
		// findMatchingRedirect -> buildReport -> writeFile against a seeded
		// .ts file. The path layout mimics coder/coder/site so repoForFile
		// classifies the finding into the expected report section.
		const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "audit-docs-cli-"));
		try {
			const redirectsPath = writeTmpRedirects(tmpDir);
			const siteRoot = path.join(tmpDir, "coder", "site", "src");
			const nested = path.join(siteRoot, "pages");
			fs.mkdirSync(nested, { recursive: true });
			const sourceFile = path.join(nested, "Example.tsx");
			fs.writeFileSync(
				sourceFile,
				'import { docs } from "./docs";\nconst href = docs("/admin/rbac");\n',
			);
			const outPath = path.join(tmpDir, "out.md");

			const errSpy = vi.spyOn(console, "error").mockImplementation(() => {});
			try {
				runCli([
					`--redirects=${redirectsPath}`,
					`--roots=${siteRoot}`,
					`--out=${outPath}`,
				]);
			} finally {
				errSpy.mockRestore();
			}

			expect(fs.existsSync(outPath)).toBe(true);
			const report = fs.readFileSync(outPath, "utf-8");

			// Summary row reflects exactly one auto-fixable finding.
			expect(report).toContain("| 1 | 1 | 0 |");
			// Repo section header for coder/coder/site.
			expect(report).toContain("## coder/coder/site");
			expect(report).toContain("1 finding.");
			// Table row includes the relative file path, the raw arg, the
			// matched redirect, and the suggested fix.
			expect(report).toContain("site/src/pages/Example.tsx:2");
			expect(report).toContain("`/admin/rbac`");
			expect(report).toContain(
				"`/docs/admin/rbac` -> `/docs/admin/templates/template-permissions`",
			);
			expect(report).toContain("`/admin/templates/template-permissions`");
		} finally {
			fs.rmSync(tmpDir, { recursive: true, force: true });
		}
	});
});
