import { describe, expect, it } from "vitest";
import {
	DOCS_LITERAL_RE,
	DOCS_TEMPLATE_RE,
	HARDCODED_URL_RE,
	MARKDOWN_LINK_RE,
	docsRedirects,
	escapeMd,
	extractReferences,
	findMatchingRedirect,
	literalPrefix,
	matchRedirect,
	relForFile,
	repoForFile,
	stripFragments,
	suggestedFixForKind,
} from "./audit_docs_paths.mjs";

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

describe("stripFragments", () => {
	it("removes a hash fragment", () => {
		expect(stripFragments("/docs/a#section")).toBe("/docs/a");
	});

	it("removes a query string", () => {
		expect(stripFragments("/docs/a?x=1")).toBe("/docs/a");
	});

	it("removes both when query precedes hash", () => {
		expect(stripFragments("/docs/a?x=1#section")).toBe("/docs/a");
	});

	it("removes both when hash precedes query", () => {
		// Slices at the first hash; query suffix after the hash is dropped too.
		expect(stripFragments("/docs/a#section?x=1")).toBe("/docs/a");
	});

	it("is a no-op when neither is present", () => {
		expect(stripFragments("/docs/a/b")).toBe("/docs/a/b");
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
	const exec = (re, content) => {
		re.lastIndex = 0;
		const out = [];
		let m;
		while ((m = re.exec(content)) !== null) out.push([...m]);
		return out;
	};

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
		expect(m.map((row) => row[1])).toEqual(["/docs/foo", "/docs/bar"]);
	});

	it("HARDCODED_URL_RE matches *.coder.com subdomains", () => {
		const src = `"https://dev.coder.com/docs/foo"`;
		const m = exec(HARDCODED_URL_RE, src);
		expect(m).toHaveLength(1);
		expect(m[0][1]).toBe("/docs/foo");
	});

	it("HARDCODED_URL_RE does NOT match markdown-link form", () => {
		// The URL is bounded by ( and ), not by quotes. This pattern misses
		// the URL; MARKDOWN_LINK_RE catches it.
		const src = `[label](https://coder.com/docs/foo)`;
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
