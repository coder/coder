import {
	formatChatSearchFilterToken,
	looksLikeChatDiffURL,
	normalizeChatDiffURLValue,
	normalizeChatSearchInput,
	resolveChatSearchFilterAlias,
} from "./searchQuery";

describe("normalizeChatSearchInput", () => {
	it("returns undefined for empty input", () => {
		expect(normalizeChatSearchInput("")).toBeUndefined();
		expect(normalizeChatSearchInput("   ")).toBeUndefined();
	});

	it("keeps key:value filters unchanged", () => {
		expect(normalizeChatSearchInput("has_unread:true")).toBe("has_unread:true");
		expect(normalizeChatSearchInput('title:"chat title" archived:true')).toBe(
			'title:"chat title" archived:true',
		);
		expect(normalizeChatSearchInput("pr_status:open,merged")).toBe(
			"pr_status:open,merged",
		);
		expect(
			normalizeChatSearchInput(
				'diff_url:"https://github.com/coder/coder/pull/25391"',
			),
		).toBe('diff_url:"https://github.com/coder/coder/pull/25391"');
	});

	it("converts bare search text into a title filter", () => {
		expect(normalizeChatSearchInput("Fix")).toBe("title:Fix");
		expect(normalizeChatSearchInput("fix auth middleware")).toBe(
			'title:"fix auth middleware"',
		);
		expect(normalizeChatSearchInput("fix:lint")).toBe('title:"fix:lint"');
	});

	it("combines key:value filters with a title fallback for bare text", () => {
		expect(normalizeChatSearchInput("has_unread:true fix auth")).toBe(
			'has_unread:true title:"fix auth"',
		);
		expect(normalizeChatSearchInput("archived:true fix:lint")).toBe(
			'archived:true title:"fix:lint"',
		);
		expect(normalizeChatSearchInput("fix has_unread:true auth")).toBe(
			'title:"fix auth" has_unread:true',
		);
		expect(
			normalizeChatSearchInput('archived:true title:"chat title" fix'),
		).toBe('archived:true title:"chat title fix"');
	});

	it("combines duplicate title filters into one title filter", () => {
		expect(normalizeChatSearchInput("title:Fix title:Race")).toBe(
			'title:"Fix Race"',
		);
		expect(
			normalizeChatSearchInput('has_unread:true title:"chat title" title:Race'),
		).toBe('has_unread:true title:"chat title Race"');
	});

	it("strips quotes from bare text", () => {
		expect(normalizeChatSearchInput('Fix "auth" middleware')).toBe(
			'title:"Fix auth middleware"',
		);
	});

	it("treats a trailing-colon filter as bare title text", () => {
		// `title:` is not a well-formed key:value pair, so it should be searched
		// for as a literal title substring.
		expect(normalizeChatSearchInput("title:")).toBe('title:"title:"');
	});

	it("routes a bare HTTPS URL into a diff_url filter", () => {
		expect(
			normalizeChatSearchInput("https://github.com/coder/coder/pull/25391"),
		).toBe('diff_url:"https://github.com/coder/coder/pull/25391"');
		expect(
			normalizeChatSearchInput("https://gitlab.com/foo/bar/-/merge_requests/9"),
		).toBe('diff_url:"https://gitlab.com/foo/bar/-/merge_requests/9"');
	});

	it("routes a scheme-less URL into a diff_url filter with https://", () => {
		expect(normalizeChatSearchInput("github.com/coder/coder/pull/25391")).toBe(
			'diff_url:"https://github.com/coder/coder/pull/25391"',
		);
	});

	it("does not mistake plain title text for a URL", () => {
		expect(normalizeChatSearchInput("coder.com")).toBe("title:coder.com");
		expect(normalizeChatSearchInput("1.2.3")).toBe("title:1.2.3");
		expect(normalizeChatSearchInput("coder/coder")).toBe("title:coder/coder");
	});

	it("quotes diff_url values that contain colons", () => {
		// The original bug: a `diff_url:` filter pill with a URL value lost its
		// quotes and produced `diff_url:https://...`, which the backend
		// rejected with a "can only contain 1 ':'" error.
		expect(
			normalizeChatSearchInput(
				"diff_url:https://github.com/coder/coder/pull/1",
			),
		).toBe('diff_url:"https://github.com/coder/coder/pull/1"');
	});

	it("auto-prepends https:// to diff_url values missing a scheme", () => {
		expect(
			normalizeChatSearchInput("diff_url:github.com/coder/coder/pull/1"),
		).toBe('diff_url:"https://github.com/coder/coder/pull/1"');
	});

	it("treats a second URL after a diff_url as title text", () => {
		expect(
			normalizeChatSearchInput(
				"https://example.com/a/1 https://example.com/b/2",
			),
		).toBe(
			'diff_url:"https://example.com/a/1" title:"https://example.com/b/2"',
		);
	});

	it("combines bare URL with other filters", () => {
		expect(
			normalizeChatSearchInput(
				"archived:true https://github.com/coder/coder/pull/1",
			),
		).toBe('archived:true diff_url:"https://github.com/coder/coder/pull/1"');
	});

	it("resolves common filter-key aliases to their canonical form", () => {
		// User typos / shorthand land on the right filter instead of falling
		// through to an always-empty title search.
		expect(normalizeChatSearchInput("archive:true")).toBe("archived:true");
		expect(normalizeChatSearchInput("unread:true")).toBe("has_unread:true");
		expect(
			normalizeChatSearchInput('diff:"https://github.com/coder/coder/pull/1"'),
		).toBe('diff_url:"https://github.com/coder/coder/pull/1"');
		expect(normalizeChatSearchInput("prstatus:open")).toBe("pr_status:open");
		expect(normalizeChatSearchInput("pr-status:open")).toBe("pr_status:open");
	});
});

describe("resolveChatSearchFilterAlias", () => {
	it.each([
		["archive", "archived"],
		["unread", "has_unread"],
		["diff", "diff_url"],
		["diffurl", "diff_url"],
		["diff-url", "diff_url"],
		["prstatus", "pr_status"],
		["pr-status", "pr_status"],
		["ARCHIVE", "archived"],
		["archived", "archived"],
		["title", "title"],
		["unknown", "unknown"],
	])("%s -> %s", (input, expected) => {
		expect(resolveChatSearchFilterAlias(input)).toBe(expected);
	});
});

describe("looksLikeChatDiffURL", () => {
	it.each([
		["https://github.com/coder/coder/pull/1", true],
		["http://example.com/foo", true],
		["github.com/coder/coder/pull/1", true],
		["gitlab.com:8080/foo/bar/-/merge_requests/9", true],
		["plain text", false],
		["coder/coder", false],
		["coder.com", false],
		["1.2.3", false],
		["", false],
		["title:foo", false],
	])("%s -> %s", (input, expected) => {
		expect(looksLikeChatDiffURL(input)).toBe(expected);
	});
});

describe("normalizeChatDiffURLValue", () => {
	it("returns http(s) URLs unchanged", () => {
		expect(
			normalizeChatDiffURLValue("https://github.com/coder/coder/pull/1"),
		).toBe("https://github.com/coder/coder/pull/1");
		expect(normalizeChatDiffURLValue("http://example.com/foo")).toBe(
			"http://example.com/foo",
		);
	});

	it("returns non-http schemed URLs unchanged so the backend can reject them", () => {
		expect(normalizeChatDiffURLValue("ftp://example.com/x")).toBe(
			"ftp://example.com/x",
		);
	});

	it("prepends https:// to scheme-less host/path values", () => {
		expect(normalizeChatDiffURLValue("github.com/coder/coder/pull/1")).toBe(
			"https://github.com/coder/coder/pull/1",
		);
	});

	it("leaves non-URL values alone", () => {
		expect(normalizeChatDiffURLValue("plain")).toBe("plain");
		expect(normalizeChatDiffURLValue("")).toBe("");
	});
});

describe("formatChatSearchFilterToken", () => {
	it("does not quote simple values", () => {
		expect(formatChatSearchFilterToken("has_unread", "true")).toBe(
			"has_unread:true",
		);
		expect(formatChatSearchFilterToken("pr_status", "open,merged")).toBe(
			"pr_status:open,merged",
		);
	});

	it("quotes values that contain colons", () => {
		expect(
			formatChatSearchFilterToken(
				"diff_url",
				"https://github.com/coder/coder/pull/1",
			),
		).toBe('diff_url:"https://github.com/coder/coder/pull/1"');
	});

	it("quotes values that contain whitespace", () => {
		expect(formatChatSearchFilterToken("title", "chat title")).toBe(
			'title:"chat title"',
		);
	});

	it("strips internal quotes before wrapping", () => {
		expect(formatChatSearchFilterToken("title", 'a "b" c')).toBe(
			'title:"a b c"',
		);
	});

	it("auto-prepends https:// for scheme-less diff_url values", () => {
		expect(
			formatChatSearchFilterToken("diff_url", "github.com/coder/coder/pull/1"),
		).toBe('diff_url:"https://github.com/coder/coder/pull/1"');
	});
});
