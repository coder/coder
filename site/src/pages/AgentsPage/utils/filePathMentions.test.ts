import { describe, expect, it } from "vitest";
import {
	findFilePathMentions,
	parseFilePathMention,
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
	])("finds mentions in %j", (text, expected) => {
		expect(mentionedText(text)).toEqual(expected);
	});

	it("reports the line suffix", () => {
		expect(findFilePathMentions("at coderd/chat.go:120:5.")).toEqual([
			{ path: "coderd/chat.go", line: 120, start: 3, end: 23 },
		]);
	});
});
