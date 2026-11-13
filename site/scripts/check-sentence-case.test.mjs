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

	it("continues a phrase across lowercase words", () => {
		expect(words("Create a Template")).toEqual(["Template"]);
		expect(words("Browse other Templates")).toEqual(["Templates"]);
		expect(words("Sign in to Coder")).toEqual([]);
		expect(words("Create a template")).toEqual([]);
	});

	it("starts a phrase only at a capitalized word", () => {
		expect(words("and Workspace Settings")).toEqual(["Settings"]);
	});

	it("reports regular words that follow an acronym", () => {
		expect(words("SSH Keys")).toEqual(["Keys"]);
	});

	it("allows proper nouns", () => {
		expect(words("Use Coder Desktop")).toEqual([]);
		expect(words("Configure AI Gateway keys")).toEqual([]);
		expect(words("Newer Anthropic-compatible endpoint")).toEqual([]);
		expect(words("Enhances Coder's parameters")).toEqual([]);
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

	it("reports title-cased segments of hyphenated words", () => {
		expect(words("Configure Role-Based Access")).toEqual([
			"Role",
			"Based",
			"Access",
		]);
		expect(words("Browser-Only connections")).toEqual(["Only"]);
		expect(words("Set X-Forwarded-For headers")).toEqual([]);
		expect(words("SFMono-Regular fonts")).toEqual([]);
		expect(words("Use built-in roles")).toEqual([]);
	});

	it("checks hyphen suffixes after proper nouns", () => {
		expect(words("Use Coder-Based Authentication")).toEqual([
			"Based",
			"Authentication",
		]);
		expect(words("Coder-Managed keys")).toEqual(["Managed"]);
		expect(findTitleCaseWords("Use Coder-Based auth")).toEqual([
			{ offset: 10, word: "Based" },
		]);
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

	it("checks template literals as one phrase", () => {
		const source = [
			"const a = `New ${label} Provider`;",
			"const b = `${message} To continue, sign in.`;",
			"const c = `${count}ms`;",
			"const d = css`font-family: Fira Code Mono;`;",
		].join("\n");
		const { issues, fixedText } = checkSource("file.ts", source);
		expect(issues.map(({ line, word }) => [line, word])).toEqual([
			[1, "Provider"],
		]);
		expect(fixedText.split("\n")[0]).toBe("const a = `New ${label} provider`;");
	});

	it("checks JSX text across inline elements as one phrase", () => {
		const source = [
			"const a = <p>Create <strong>Workspace</strong></p>;",
			"const b = <p>Create <strong>New Workspace</strong></p>;",
			'const c = <p>Create{" "}',
			"\t<strong>Template</strong>",
			"</p>;",
		].join("\n");
		const { issues, fixedText } = checkSource("file.tsx", source);
		expect(issues.map(({ line, word }) => [line, word])).toEqual([
			[1, "Workspace"],
			[2, "New"],
			[2, "Workspace"],
			[4, "Template"],
		]);
		expect(fixedText.split("\n")[1]).toBe(
			"const b = <p>Create <strong>new workspace</strong></p>;",
		);
	});

	it("checks JSX rendered by conditional expressions in place", () => {
		const source = [
			"const a = <p>Create {show && <strong>Workspace</strong>}</p>;",
			'const b = <p>Create {show ? <b>New Template</b> : "Old Template"}</p>;',
			"const c = <p>Name {value}</p>;",
		].join("\n");
		const { issues, fixedText } = checkSource("file.tsx", source);
		expect(issues.map(({ line, word }) => [line, word])).toEqual([
			[1, "Workspace"],
			[2, "New"],
			[2, "Template"],
			[2, "Old"],
			[2, "Template"],
		]);
		expect(fixedText.split("\n")[1]).toBe(
			'const b = <p>Create {show ? <b>new template</b> : "old template"}</p>;',
		);
	});

	it("checks fallback values rendered in JSX in place", () => {
		const source = [
			'const a = <p>Create {label || "New Workspace"}</p>;',
			'const b = <p>Create {label ?? "New Template"}</p>;',
		].join("\n");
		const { issues, fixedText } = checkSource("file.tsx", source);
		expect(issues.map(({ line, word }) => [line, word])).toEqual([
			[1, "New"],
			[1, "Workspace"],
			[2, "New"],
			[2, "Template"],
		]);
		expect(fixedText.split("\n")[0]).toBe(
			'const a = <p>Create {label || "new workspace"}</p>;',
		);
	});

	it("lowercases title-cased segments of hyphenated words", () => {
		const { fixedText } = checkSource(
			"file.tsx",
			'const a = "Configure Role-Based Access";',
		);
		expect(fixedText).toBe('const a = "Configure role-based access";');
	});

	it("checks template literals rendered in JSX in place", () => {
		const source = [
			"const a = <p>Create {`New ${kind} Workspace`}</p>;",
			"const b = <p>Create {show ? `New ${kind}` : null}</p>;",
			"const c = <p>Retry {`${count}s`}</p>;",
		].join("\n");
		const { issues, fixedText } = checkSource("file.tsx", source);
		expect(issues.map(({ line, word }) => [line, word])).toEqual([
			[1, "New"],
			[1, "Workspace"],
			[2, "New"],
		]);
		expect(fixedText.split("\n")[0]).toBe(
			"const a = <p>Create {`new ${kind} workspace`}</p>;",
		);
	});

	it("decodes HTML entities in JSX text and attributes", () => {
		const source = [
			"const a = <p>Test Validate&hellip;</p>;",
			'const b = <Button title="Delete&nbsp;File" />;',
			"const c = <p>Click Don&apos;t</p>;",
		].join("\n");
		const { issues, fixedText } = checkSource("file.tsx", source);
		expect(issues.map(({ line, word }) => [line, word])).toEqual([
			[1, "Validate"],
			[2, "File"],
			[3, "Don't"],
		]);
		expect(fixedText.split("\n")[0]).toBe(
			"const a = <p>Test validate&hellip;</p>;",
		);
	});

	it("starts a new phrase at elements without a rendered space", () => {
		const source = [
			"const a = (",
			"\t<div>",
			"\t\t<h2>Settings </h2>",
			"\t\t<Button>Save</Button>",
			"\t\t<span>",
			"\t\t\t<PlayIcon /> Start",
			"\t\t</span>",
			"\t</div>",
			");",
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
