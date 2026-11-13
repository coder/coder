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

	it("handles accented capitals and lowercase letters", () => {
		expect(words("Open Résumé Builder")).toEqual(["Résumé", "Builder"]);
		expect(words("Open Re\u0301sume\u0301 Builder")).toEqual([
			"Re\u0301sume\u0301",
			"Builder",
		]);
		expect(words("Édit café settings")).toEqual([]);
	});

	it("reports the article A but not single-letter names", () => {
		expect(words("Create A workspace")).toEqual(["A"]);
		expect(words("Press the left arrow or A to rotate")).toEqual([]);
		expect(words("Use plan A or B")).toEqual([]);
		expect(words("Choose option A")).toEqual([]);
	});

	it("reports regular words that follow an acronym", () => {
		expect(words("SSH Keys")).toEqual(["Keys"]);
	});

	it("allows proper nouns", () => {
		expect(words("Use Coder Desktop")).toEqual([]);
		expect(words("Configure AI Gateway keys")).toEqual([]);
		expect(words("Check Workspace Proxy health")).toEqual([]);
		expect(words("Open the Web Terminal")).toEqual([]);
		expect(words("Newer Anthropic-compatible endpoint")).toEqual([]);
		expect(words("Enhances Coder's parameters")).toEqual([]);
	});

	it("starts a new phrase after sentence or list punctuation", () => {
		expect(words("Saved. Next steps follow.")).toEqual([]);
		expect(words("Warning: Read this")).toEqual([]);
		expect(words("GitHub, GitLab, Bitbucket")).toEqual([]);
		expect(words("Fonts: Menlo, Monaco, Consolas")).toEqual([]);
		expect(words("Saved… Next step")).toEqual([]);
		expect(words("Saved “done.” Next step")).toEqual([]);
		expect(words("Saved {done.} Next step")).toEqual([]);
	});

	it("continues a phrase across connectors but not separators", () => {
		expect(words("Create & Manage workspaces")).toEqual(["Manage"]);
		expect(words("Create / Manage workspaces")).toEqual(["Manage"]);
		expect(words("Workspace proxy - Health")).toEqual([]);
		expect(words("Settings | Coder")).toEqual([]);
	});

	it("continues a sentence after a comma", () => {
		expect(words("Create a workspace, then Open settings")).toEqual(["Open"]);
		expect(words("Create a workspace, Then open settings")).toEqual(["Then"]);
		expect(words("Create a workspace; Then open settings")).toEqual(["Then"]);
	});

	it("resumes the surrounding phrase after a bracket or quote closes", () => {
		expect(words("Create a (temporary) Workspace")).toEqual(["Workspace"]);
		expect(words('Pick "admin" Access')).toEqual(["Access"]);
		expect(words("Default (Recommended) Option")).toEqual(["Option"]);
		expect(words("Saved (see logs.) Next steps")).toEqual([]);
		expect(words("Create a ‘temporary’ Workspace")).toEqual(["Workspace"]);
		expect(words("Create a 'temporary' Workspace")).toEqual(["Workspace"]);
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
			'if (event === ("Task Events" as const)) {}',
			'if (event === "Task Events" + suffix) {}',
			"if (event === `Task Events ${suffix}`) {}",
			'switch (group) { case "User Events"!: break; }',
			'enum Event { Task = "Task Events" }',
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

	it("reads through type-only wrappers in JSX", () => {
		const source = [
			'const a = <p>Create {"New Workspace" as const}</p>;',
			'const b = <p>Create {("New Template" satisfies string)}</p>;',
			'const c = <p>Create {label! || "New Group"}</p>;',
		].join("\n");
		const { issues } = checkSource("file.tsx", source);
		expect(issues.map(({ line, word }) => [line, word])).toEqual([
			[1, "New"],
			[1, "Workspace"],
			[2, "New"],
			[2, "Template"],
			[3, "New"],
			[3, "Group"],
		]);
	});

	it("checks arrays rendered in JSX in place", () => {
		const source = 'const a = <p>Create {["New ", "Workspace"]}</p>;';
		const { issues } = checkSource("file.tsx", source);
		expect(issues.map(({ word }) => word)).toEqual(["New", "Workspace"]);
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

	it("checks string concatenation as one phrase, per rendered variant", () => {
		const source = [
			'const a = <p>Create {"New " + "Workspace"}</p>;',
			'const b = "Create " + "New Workspace";',
			'const c = "Retry in " + count + " Seconds";',
			"const d = total + 1;",
			'const e = "Create " + (ok ? "New Workspace" : "Old Template");',
			'const f = `${ok ? "New" : "Old"} Workspace`;',
		].join("\n");
		const { issues, fixedText } = checkSource("file.tsx", source);
		expect(issues.map(({ line, word }) => [line, word])).toEqual([
			[1, "New"],
			[1, "Workspace"],
			[2, "New"],
			[2, "Workspace"],
			[3, "Seconds"],
			[5, "New"],
			[5, "Workspace"],
			[5, "Old"],
			[5, "Template"],
			[6, "Workspace"],
		]);
		expect(fixedText.split("\n")[0]).toBe(
			'const a = <p>Create {"new " + "workspace"}</p>;',
		);
	});

	it("decodes HTML entities in JSX text and attributes", () => {
		const source = [
			"const a = <p>Test Validate&hellip;</p>;",
			'const b = <Button title="Delete&nbsp;File" />;',
			"const c = <p>Click Don&apos;t</p>;",
			"const d = <p>Saved&hellip; Next step</p>;",
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

	it("decodes escapes in string and template literals", () => {
		const source = [
			'const a = "Create\\nNew Workspace";',
			"const b = `Create\\tNew ${kind}`;",
			'const c = "Create \\u0057orkspace";',
			'const d = <Button title="Delete\\nFile" />;',
		].join("\n");
		const { issues, fixedText } = checkSource("file.tsx", source);
		expect(issues.map(({ line, word }) => [line, word])).toEqual([
			[1, "New"],
			[1, "Workspace"],
			[2, "New"],
			[3, "Workspace"],
		]);
		expect(fixedText.split("\n").slice(0, 3)).toEqual([
			'const a = "Create\\nnew workspace";',
			"const b = `Create\\tnew ${kind}`;",
			'const c = "Create workspace";',
		]);
	});

	it("lowercases capitals outside the Basic Multilingual Plane", () => {
		const { fixedText } = checkSource(
			"file.ts",
			'const a = "Open 𐐀est";\nconst b = "Open \\uD801\\uDC00est";',
		);
		expect(fixedText).toBe('const a = "Open 𐐨est";\nconst b = "Open 𐐨est";');
	});

	it("fixes words that start with an HTML entity", () => {
		const source = "const a = <p>Create &#87;orkspace</p>;";
		const { issues, fixedText } = checkSource("file.tsx", source);
		expect(issues.map(({ word }) => word)).toEqual(["Workspace"]);
		expect(fixedText).toBe("const a = <p>Create workspace</p>;");
	});

	it("joins elements across an encoded space", () => {
		const source = [
			"const a = <p>Create&nbsp;<b>New Workspace</b></p>;",
			"const b = <p>Create&emsp;<b>New Template</b></p>;",
		].join("\n");
		const { issues } = checkSource("file.tsx", source);
		expect(issues.map(({ word }) => word)).toEqual([
			"New",
			"Workspace",
			"New",
			"Template",
		]);
	});

	it("decodes every named entity JSX supports", () => {
		const { issues, fixedText } = checkSource(
			"file.tsx",
			"const a = <p>Create &Eacute;dit Workspace</p>;",
		);
		expect(issues.map(({ word }) => word)).toEqual(["Édit", "Workspace"]);
		expect(fixedText).toBe("const a = <p>Create édit workspace</p>;");
	});

	it("skips attributes that are not displayed text", () => {
		const source = [
			'const a = <div data-state="New Workspace" className="Foo Bar" />;',
			'const b = <div aria-controls="Some Id" id="Main Panel" />;',
			'const c = <div data-state={open ? "Is Open" : "Is Closed"} />;',
			'const d = <input aria-label="Search Workspaces" title="Delete File" />;',
			'const e = <option value="New Workspace">New Template</option>;',
			'const f = <div className={cn("Foo Bar", { "Baz Qux": ok })} />;',
			"const g = <div style={{ gridTemplateAreas: '\"Main Header\"' }} />;",
			'const h = <div data-values={items.map(() => "New Workspace")} />;',
			'const i = <td headers="Plan Name" />;',
			'const j = <div {...{ className: "Foo Bar", "data-x": "Is On" }} />;',
			'const k = <Card className="Foo Bar" value="Some Value" />;',
			'const n = <div {...(ok ? { className: "Foo Bar" } : {})} />;',
			'const o = <div {...(ok && { "data-x": "Is On" })} />;',
			'const l = <input name="Plan Name" />;',
			'const m = <ModuleCard name="New Workspace" />;',
		].join("\n");
		const { issues } = checkSource("file.tsx", source);
		expect(issues.map(({ line, word }) => [line, word])).toEqual([
			[4, "Workspaces"],
			[4, "File"],
			[5, "Template"],
			[15, "Workspace"],
		]);
	});

	it("ends the phrase at block elements", () => {
		const source = [
			"const a = <div>Configure <h2>Workspace Settings</h2></div>;",
			"const b = <div><p>Saved </p> <p>Next Steps</p></div>;",
			"const c = <p>Create <label>New Workspace</label></p>;",
			"const d = <p>Click <button>Save Changes</button></p>;",
		].join("\n");
		const { issues } = checkSource("file.tsx", source);
		expect(issues.map(({ line, word }) => [line, word])).toEqual([
			[1, "Settings"],
			[2, "Steps"],
			[3, "New"],
			[3, "Workspace"],
			[4, "Changes"],
		]);
	});

	it("joins inline sibling elements by the space between them", () => {
		const source = [
			"const a = <p><span>Create </span><strong>Workspace</strong></p>;",
			"const b = <p><span>Create</span><strong>Workspace</strong></p>;",
		].join("\n");
		const { issues } = checkSource("file.tsx", source);
		expect(issues.map(({ line, word }) => [line, word])).toEqual([
			[1, "Workspace"],
		]);
	});

	it("reads code elements as unknown text", () => {
		const source = [
			"const a = <p>Send the header, for example <code>Bearer Token</code>.</p>;",
			'const b = <code>{"New Workspace"}</code>;',
			"const c = <kbd><span>Ctrl Shift</span></kbd>;",
			'const d = <code aria-label="New Workspace">value</code>;',
			'const e = <pre title="Delete File">{"Some Code"}</pre>;',
			"const f = <div>Run <pre>cmd</pre>Next Steps</div>;",
			'const g = <pre><button aria-label="New Workspace">Copy Code</button></pre>;',
			'const h = <pre>{ok && <span title="Show Details">Some Code</span>}</pre>;',
		].join("\n");
		const { issues } = checkSource("file.tsx", source);
		expect(issues.map(({ line, word }) => [line, word])).toEqual([
			[4, "Workspace"],
			[5, "File"],
			[6, "Steps"],
			[7, "Workspace"],
			[8, "Details"],
		]);
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
