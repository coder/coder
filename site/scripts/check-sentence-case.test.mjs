import { describe, expect, it } from "vitest";
import { checkSource, findTitleCaseWords } from "./check-sentence-case.mjs";

const words = (text) => findTitleCaseWords(text).map(({ word }) => word);

describe("findTitleCaseWords", () => {
	it("reports capitalized words after the first word of a phrase", () => {
		expect(words("Create New Workspace")).toEqual(["New", "Workspace"]);
	});

	it("allows sentence case", () => {
		expect(words("Create new workspace")).toEqual([]);
	});

	it("allows acronyms and mixed-case names", () => {
		expect(words("SSH keys for GitHub")).toEqual([]);
		expect(words("Open in VS Code")).toEqual([]);
	});

	it("reports regular words that follow an acronym", () => {
		expect(words("SSH Keys")).toEqual(["Keys"]);
	});

	it("allows proper nouns", () => {
		expect(words("Use Coder Desktop")).toEqual([]);
		expect(words("Configure AI Gateway keys")).toEqual([]);
		expect(words("Newer Anthropic-compatible endpoint")).toEqual([]);
	});

	it("starts a new phrase after sentence or list punctuation", () => {
		expect(words("Saved. Next steps follow.")).toEqual([]);
		expect(words("Warning: Read this")).toEqual([]);
		expect(words("GitHub, GitLab, Bitbucket")).toEqual([]);
	});

	it("starts a new phrase after an opening bracket or quote", () => {
		expect(words('Pick "Admin" access')).toEqual([]);
		expect(words("Default (Recommended)")).toEqual([]);
	});

	it("returns the offset of each reported word", () => {
		expect(findTitleCaseWords("Delete  Workspace")).toEqual([
			{ offset: 8, word: "Workspace" },
		]);
	});
});

describe("checkSource", () => {
	it("checks JSX text and string literals", () => {
		const source = [
			"const a = <h1>Workspace Settings</h1>;",
			'const b = <Button title="Delete File" />;',
		].join("\n");
		const { issues } = checkSource("file.tsx", source);
		expect(issues.map(({ line, word }) => [line, word])).toEqual([
			[1, "Settings"],
			[2, "File"],
		]);
	});

	it("skips literals that are not displayed text", () => {
		const source = [
			'import { Thing } from "Some Module";',
			'type Group = "Task Events";',
			'const obj = { "Some Key": 1 };',
			'if (group === "Task Events") {}',
			'switch (group) { case "User Events": break; }',
			'const value = lookup["Other Key"];',
		].join("\n");
		expect(checkSource("file.tsx", source).issues).toEqual([]);
	});

	it("lowercases reported words in the fixed text", () => {
		const { fixedText } = checkSource(
			"file.tsx",
			'const a = "Create New Workspace";',
		);
		expect(fixedText).toBe('const a = "Create new workspace";');
	});

	it("honors expect comments on the previous line", () => {
		const source = [
			"// sentence-case-expect: API value",
			'const a = "Startup Script";',
		].join("\n");
		const { issues, unusedExpectations, fixedText } = checkSource(
			"file.ts",
			source,
		);
		expect(issues).toEqual([]);
		expect(unusedExpectations).toEqual([]);
		expect(fixedText).toBe(source);
	});

	it("reports expect comments that suppress nothing", () => {
		const source = [
			"const a = 1;",
			"// sentence-case-expect: stale",
			'const b = "Already sentence case";',
		].join("\n");
		expect(checkSource("file.ts", source).unusedExpectations).toEqual([2]);
	});
});
