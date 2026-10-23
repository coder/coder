import { describe, expect, it } from "vitest";
import {
	findCandidateTokenStarts,
	findFilePathCandidates,
	findFilePathMentions,
	findQuotedPathEnd,
	getFilePathCandidateStarts,
	isWholeFilePathCandidate,
	parseFilePathMention,
	walkFilePath,
} from "./filePathMentions";

describe("parseFilePathMention", () => {
	it.each([
		["main.go", { path: "main.go" }],
		["README.md", { path: "README.md" }],
		["Makefile", { path: "Makefile" }],
		["docker/Dockerfile", { path: "docker/Dockerfile" }],
		["site/src/api/api.ts", { path: "site/src/api/api.ts" }],
		["/home/coder/coder/go.mod", { path: "/home/coder/coder/go.mod" }],
		["~/notes/todo.txt", { path: "~/notes/todo.txt" }],
		["./scripts/develop.sh", { path: "./scripts/develop.sh" }],
		["../other/x.vue", { path: "../other/x.vue" }],
		[".github/workflows/ci.yaml", { path: ".github/workflows/ci.yaml" }],
		["coderd/chat.go:120", { path: "coderd/chat.go", line: 120 }],
		["coderd/chat.go:120:5", { path: "coderd/chat.go", line: 120 }],
		["coderd/chat.go#L42", { path: "coderd/chat.go", line: 42 }],
		["src/app/(auth)/page.tsx", { path: "src/app/(auth)/page.tsx" }],
		["src/[id]/page.tsx:7", { path: "src/[id]/page.tsx", line: 7 }],
		[
			"node_modules/@types/node/index.d.ts",
			{ path: "node_modules/@types/node/index.d.ts" },
		],
		[" api.ts ", { path: "api.ts" }],
	])("parses %j", (text, expected) => {
		expect(parseFilePathMention(text)).toEqual(expected);
	});

	it.each([
		"v1.2.3",
		"1.2",
		"example.com",
		"example.com/docs/index.html",
		"https://github.com/coder/coder/blob/main/go.mod",
		"user@example.com",
		"node_modules/react",
		"main.exe",
		"foo.",
		".go",
		"and/or",
		"2024/01.02",
		"fmt.Println",
		"git status",
		"main.go:abc",
		"Node.js",
		"docs.rs",
		"user@example.md",
		"src/my dir/a.ts",
		"C:\\Users\\me\\a.ts",
		"",
	])("rejects %j", (text) => {
		expect(parseFilePathMention(text)).toBeUndefined();
	});
});

describe("findFilePathMentions", () => {
	const mentionedText = (text: string) =>
		findFilePathMentions(text).map(({ start, end }) => text.slice(start, end));

	it.each([
		["Edit main.go and api.ts.", ["main.go", "api.ts"]],
		["See (site/src/App.tsx:12), then rerun.", ["site/src/App.tsx:12"]],
		["Open /etc/hosts.txt now", ["/etc/hosts.txt"]],
		["Check ~/x/y.md#L3!", ["~/x/y.md#L3"]],
		["Run the Makefile target", ["Makefile"]],
		["Bumped to v1.2.3 on example.com", []],
		["Go to https://example.com/a/b.go for details", []],
		["Mail user@example.com", []],
		["Visit www.example.com/index.html", []],
		["Nothing here", []],
		["Built with Node.js and Next.js", []],
		["See docs.rs or install bun.sh", []],
		["Mail user@example.md", []],
		["Edit src/[id]/page.tsx", []],
		["Edit src/app/(auth)/page.tsx", []],
		["Edit C:\\Users\\me\\app.ts", []],
		["Edit C:/Users/me/app.ts", []],
		["Edit /home/me/a.ts", ["/home/me/a.ts"]],
	])("finds mentions in %j", (text, expected) => {
		expect(mentionedText(text)).toEqual(expected);
	});

	it("reports the line suffix", () => {
		expect(findFilePathMentions("at coderd/chat.go:120:5.")).toEqual([
			{ path: "coderd/chat.go", line: 120, start: 3, end: 23 },
		]);
	});
});

describe("findFilePathCandidates", () => {
	const candidateText = (text: string) =>
		findFilePathCandidates(text).map(({ start, end }) =>
			text.slice(start, end),
		);

	it.each([
		["Open /path/to/file (2).txt now", ["/path/to/file (2).txt now"]],
		["In /home/coder/my dir/a.ts.\nNext line", ["/home/coder/my dir/a.ts."]],
		["Edit src/my dir/a.ts please", ["src/my dir/a.ts please"]],
		["See ~/My Documents/x.md", ["~/My Documents/x.md"]],
		["Edit site/src/a.ts and b.go", ["site/src/a.ts and b.go"]],
		["Edit a.ts and b.go", []],
		["Ends with /home/coder/a.ts", []],
		["Ends with /home/coder/a.ts\nnext", []],
		["Visit https://example.com/my page", []],
		["Visit example.com/my page", []],
		["Edit C:/Users/my dir/a.ts", []],
		["Two /a/b c\n/d/e f", ["/a/b c", "/d/e f"]],
		["See site/src/pages/ for routes", ["site/src/pages/ for routes"]],
		["Look in docs/.", ["docs/."]],
		["Ends with /etc/", ["/etc/"]],
		["Visit https://example.com/ or example.com/a/", []],
		["Read site/src/a.ts/ now", ["site/src/a.ts/ now"]],
	])("finds candidates in %j", (text, expected) => {
		expect(candidateText(text)).toEqual(expected);
	});

	it("caps the tail and covers a mention it overlaps", () => {
		const text = `/a/b ${"x".repeat(155)} c/d.go end`;
		expect(candidateText(text)).toEqual([text.slice(0, text.length - 4)]);
	});
});

describe("isWholeFilePathCandidate", () => {
	it.each([
		["/path/to/file (2).txt", true],
		["my dir/a.ts", true],
		["~/My Documents/x.md", true],
		["site/src/pages/", true],
		["/home/coder/a.ts", false],
		["git status", false],
		['git commit -m "fix/thing x"', false],
		["curl https://example.com/a b", false],
	])("classifies %j", (text, expected) => {
		expect(isWholeFilePathCandidate(text)).toBe(expected);
	});
});

describe("getFilePathCandidateStarts", () => {
	it.each([
		[
			"/path/to/file (2).txt",
			[],
			[{ directory: "/path/to", offset: 9, root: "/" }],
		],
		["/file (2).txt", [], [{ directory: "/", offset: 1, root: "/" }]],
		["~/My Documents/x.md", [], [{ directory: "~", offset: 2, root: "~" }]],
		["~/a/my dir/x.md", [], [{ directory: "~/a", offset: 4, root: "~" }]],
		[
			"src/my dir/a.ts",
			["/r1", "/r2/"],
			[
				{ directory: "/r1/src", offset: 4, root: "/r1" },
				{ directory: "/r2/src", offset: 4, root: "/r2/" },
			],
		],
		["my dir/a.ts", [], [{ directory: "", offset: 0, root: "" }]],
		["src/my dir/a.ts", [], [{ directory: "src", offset: 4, root: "" }]],
		[
			"site/src/pages/ for routes",
			["/r1"],
			[{ directory: "/r1/site/src", offset: 9, root: "/r1" }],
		],
		["/home/", [], [{ directory: "/", offset: 1, root: "/" }]],
		["~/projects/", [], [{ directory: "~", offset: 2, root: "~" }]],
	])("starts %j in %j", (text, roots, expected) => {
		expect(getFilePathCandidateStarts(text, roots)).toEqual(expected);
	});
});

describe("walkFilePath", () => {
	const tree: Record<string, string[] | null> = {
		"/path/to": ["file", "file (2).txt", "file (2)"],
		"/home/coder": ["my", "my dir/"],
		"/home/coder/my dir": ["a.ts"],
		"/repo": ["my/"],
		"/missing": null,
	};
	const getEntries = (directory: string) => {
		const names = tree[directory];
		if (names === undefined || names === null) {
			return names;
		}
		return names.map((name) => {
			const isDir = name.endsWith("/");
			const entryName = isDir ? name.slice(0, -1) : name;
			return {
				name: entryName,
				is_dir: isDir,
				absolute_path_string: `${directory}/${entryName}`,
			};
		});
	};

	it.each([
		[
			"/path/to",
			"file (2).txt now",
			{ absolutePath: "/path/to/file (2).txt", length: 12 },
		],
		[
			"/path/to",
			"file (2).txt:7, then",
			{ absolutePath: "/path/to/file (2).txt", length: 14, line: 7 },
		],
		["/path/to", "file and more", { absolutePath: "/path/to/file", length: 4 }],
		[
			"/home/coder",
			"my dir/a.ts.",
			{ absolutePath: "/home/coder/my dir/a.ts", length: 11 },
		],
		["/home/coder", "my dir/b.ts", null],
		["/home/coder", "my dirt", { absolutePath: "/home/coder/my", length: 2 }],
		[
			"/home/coder",
			"my dir/ holds it",
			{ absolutePath: "/home/coder/my dir", length: 7, isDirectory: true },
		],
		[
			"/home/coder",
			"my dir, which",
			{ absolutePath: "/home/coder/my dir", length: 6, isDirectory: true },
		],
		[
			"/home/coder",
			"my dir/",
			{ absolutePath: "/home/coder/my dir", length: 7, isDirectory: true },
		],
		["/missing", "a b.ts", null],
		["/repo", 'my docs/missing/" now', null],
		["/home/coder", "my notes/a.md", null],
		[
			"/repo",
			"my /etc/hosts",
			{ absolutePath: "/repo/my", length: 2, isDirectory: true },
		],
		["/not/listed/yet", "a b.ts", undefined],
	])("walks from %j through %j", (directory, text, expected) => {
		expect(walkFilePath(directory, text, getEntries).verified).toEqual(
			expected,
		);
	});

	it("reports the directories it needed, up to the pending one", () => {
		expect(
			walkFilePath("/home/coder", "my dir/sub/a.ts", (directory) =>
				directory === "/home/coder/my dir/sub"
					? undefined
					: getEntries(directory),
			),
		).toEqual({
			directories: ["/home/coder", "/home/coder/my dir"],
			verified: null,
		});
	});

	it("stops at the depth cap", () => {
		const walk = walkFilePath("/", "d/d/d/d/d/d/a.ts", (directory) => [
			{ name: "d", is_dir: true, absolute_path_string: `${directory}/d` },
		]);
		expect(walk.verified).toBeNull();
		expect(walk.directories).toHaveLength(5);
	});
});

describe("findQuotedPathEnd", () => {
	it.each([
		['/repo/my docs" now', '"', 13],
		["/repo/my docs” now", "“", 13],
		["/repo/my docs now", '"', -1],
		['/repo/my docs" now', " ", -1],
		['/repo/my docs" now', undefined, -1],
	])("ends %j opened by %j at %d", (text, opener, expected) => {
		expect(findQuotedPathEnd(text, opener)).toBe(expected);
	});
});

describe("findCandidateTokenStarts", () => {
	const tokens = (text: string) =>
		findCandidateTokenStarts(text).map((start) =>
			text.slice(start, text.indexOf(" ", start)),
		);

	it.each([
		[
			"/r/ux-notes/my notes/notes (2).md and in ux-notes/my notes/notes (2).md as well.",
			["/r/ux-notes/my", "notes/notes", "ux-notes/my", "notes/notes"],
		],
		["Edit a.ts and b.go", []],
		["Visit https://example.com/my page or example.com/a b", []],
	])("lists the starts in %j", (text, expected) => {
		expect(tokens(text)).toEqual(expected);
	});
});
