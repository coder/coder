import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	buildPersonalSkillMarkdown,
	filterSkillsByQuery,
	getPersonalSkillContentSizeBytes,
	isPersonalSkillTriggerToken,
	isValidPersonalSkillDescription,
	isValidPersonalSkillName,
	PERSONAL_SKILL_MAX_SIZE_BYTES,
	parsePersonalSkillMarkdown,
	parsePersonalSkillTrigger,
	personalSkillTriggerText,
	tryParsePersonalSkillMarkdown,
} from "./personalSkills";

const now = "2026-05-08T00:00:00Z";

const skill = (
	name: string,
	description: string,
	index: number,
): TypesGen.UserSkillMetadata => ({
	id: `skill-${index}`,
	name,
	description,
	created_at: now,
	updated_at: now,
});

describe("filterSkillsByQuery", () => {
	const skills = [
		skill("deploy", "Ship reviewed production changes", 0),
		skill("reviewer", "Review changed files", 1),
		skill("docs", "Draft deployment docs", 2),
		skill("api-review", "Review API changes", 3),
	];

	it("sorts unfiltered skills by name", () => {
		expect(filterSkillsByQuery(skills, "").map(({ name }) => name)).toEqual([
			"api-review",
			"deploy",
			"docs",
			"reviewer",
		]);
	});

	it("ranks prefix, name substring, then description matches", () => {
		expect(filterSkillsByQuery(skills, "rev").map(({ name }) => name)).toEqual([
			"reviewer",
			"api-review",
			"deploy",
		]);
	});

	it("matches names and descriptions case-insensitively", () => {
		const mixedCaseSkills = [
			skill("deploy-bot", "Ship Changes", 0),
			skill("docs", "Review docs", 1),
		];

		expect(
			filterSkillsByQuery(mixedCaseSkills, "DEP").map(({ name }) => name),
		).toEqual(["deploy-bot"]);
		expect(
			filterSkillsByQuery(mixedCaseSkills, "changes").map(({ name }) => name),
		).toEqual(["deploy-bot"]);
	});

	it("matches trigger text", () => {
		const skillsWithTriggerText = [
			{
				name: "reviewer",
				description: "Review changed files",
				triggerText: "/reviewer",
			},
			{
				name: "test-runner",
				description: "Run tests",
				triggerText: "/workspace/test-runner",
			},
		];

		expect(
			filterSkillsByQuery(skillsWithTriggerText, "workspace/t").map(
				({ triggerText }) => triggerText,
			),
		).toEqual(["/workspace/test-runner"]);
	});

	it("matches the alternate qualified trigger of a bare item", () => {
		const skills = [
			{
				name: "reviewer",
				description: "Review changed files",
				triggerText: "/reviewer",
				altTriggerText: "/personal/reviewer",
			},
		];

		expect(
			filterSkillsByQuery(skills, "personal/rev").map(({ name }) => name),
		).toEqual(["reviewer"]);
	});
});

describe("personal skill slash triggers", () => {
	it("formats skill trigger text", () => {
		expect(personalSkillTriggerText(skill("reviewer", "", 0))).toBe(
			"/reviewer",
		);
	});

	it("parses trigger text at line start or after whitespace", () => {
		expect(parsePersonalSkillTrigger("/rev")).toEqual({
			slashOffset: 0,
			query: "rev",
		});
		expect(parsePersonalSkillTrigger("ask /docs")).toEqual({
			slashOffset: 4,
			query: "docs",
		});
	});

	it("rejects mid-token slash triggers", () => {
		expect(parsePersonalSkillTrigger("https://")).toBeNull();
	});

	it("validates replacement trigger tokens", () => {
		expect(isPersonalSkillTriggerToken("/rev")).toBe(true);
		expect(isPersonalSkillTriggerToken("/bad token")).toBe(false);
	});
});

describe("parsePersonalSkillMarkdown", () => {
	it("parses SKILL.md frontmatter and body", () => {
		expect(
			parsePersonalSkillMarkdown(
				'---\nname: "test-skill"\ndescription: "Does a thing"\n---\n\nUse this skill.',
			),
		).toEqual({
			name: "test-skill",
			description: "Does a thing",
			body: "Use this skill.",
		});
	});

	it("parses folded YAML description values", () => {
		expect(
			parsePersonalSkillMarkdown(
				[
					"---",
					"name: brainstorming",
					"description: >",
					"  Use before any creative work: features, components, functionality changes,",
					"  or behavior modifications. Turns ideas into approved designs through",
					"  collaborative dialog. Hard gate: no implementation action until the",
					"  design is presented and approved.",
					"---",
					"Use this skill.",
				].join("\n"),
			),
		).toEqual({
			name: "brainstorming",
			description: [
				"Use before any creative work: features, components, functionality changes,",
				"or behavior modifications. Turns ideas into approved designs through",
				"collaborative dialog. Hard gate: no implementation action until the",
				"design is presented and approved.",
			].join(" "),
			body: "Use this skill.",
		});
	});

	it("uses YAML comment semantics in frontmatter", () => {
		expect(
			parsePersonalSkillMarkdown(
				"---\nname: test-skill\ndescription: Build # test\n---\nBody",
			),
		).toEqual({
			name: "test-skill",
			description: "Build",
			body: "Body",
		});
	});

	it("rejects non-string frontmatter fields", () => {
		expect(() =>
			parsePersonalSkillMarkdown("---\nname: null\n---\nBody"),
		).toThrow("Skill name must be a string.");
		expect(() =>
			parsePersonalSkillMarkdown(
				"---\nname: test-skill\ndescription: null\n---\nBody",
			),
		).toThrow("Skill description must be a string.");
	});

	it("allows whitespace around frontmatter delimiters", () => {
		expect(
			parsePersonalSkillMarkdown("  ---  \nname: test-skill\n  ---  \nBody"),
		).toEqual({
			name: "test-skill",
			description: "",
			body: "Body",
		});
	});

	it("requires a non-empty body", () => {
		expect(() =>
			parsePersonalSkillMarkdown("---\nname: test-skill\n---\n\n"),
		).toThrow("Skill body is required.");
	});

	it("preserves HTML comments in the body", () => {
		expect(
			parsePersonalSkillMarkdown(
				"---\nname: test-skill\n---\n\nKeep <!-- TODO --> notes.",
			),
		).toMatchObject({
			body: "Keep <!-- TODO --> notes.",
		});
	});
});

describe("tryParsePersonalSkillMarkdown", () => {
	it("returns parsed values for valid SKILL.md content", () => {
		expect(
			tryParsePersonalSkillMarkdown(
				"---\nname: test-skill\ndescription: Does a thing\n---\nBody",
			),
		).toEqual({
			ok: true,
			values: {
				name: "test-skill",
				description: "Does a thing",
				body: "Body",
			},
		});
	});

	it("returns an error message for invalid SKILL.md content", () => {
		expect(
			tryParsePersonalSkillMarkdown(
				"---\ndescription: Missing name\n---\nBody",
			),
		).toEqual({
			ok: false,
			error: "Skill name is required.",
		});
	});

	it("keeps delimiter errors distinct", () => {
		expect(
			tryParsePersonalSkillMarkdown("name: test-skill\n---\nBody"),
		).toEqual({
			ok: false,
			error: "Missing opening frontmatter delimiter.",
		});
		expect(
			tryParsePersonalSkillMarkdown("---\nname: test-skill\nBody"),
		).toEqual({
			ok: false,
			error: "Missing closing frontmatter delimiter.",
		});
	});
});

describe("buildPersonalSkillMarkdown", () => {
	it("builds backend-compatible skill markdown", () => {
		const content = buildPersonalSkillMarkdown({
			name: "test-skill",
			description: "Does a thing",
			body: "Use this skill.",
		});

		expect(content).toBe(
			'---\nname: test-skill\ndescription: "Does a thing"\n---\nUse this skill.\n',
		);
		expect(parsePersonalSkillMarkdown(content)).toEqual({
			name: "test-skill",
			description: "Does a thing",
			body: "Use this skill.",
		});
	});

	it("omits the description line when description is empty", () => {
		expect(
			buildPersonalSkillMarkdown({
				name: "test-skill",
				description: "",
				body: "Use this skill.",
			}),
		).toBe("---\nname: test-skill\n---\nUse this skill.\n");
	});

	it("quotes skill names that YAML would otherwise coerce", () => {
		const content = buildPersonalSkillMarkdown({
			name: "true",
			description: "",
			body: "Use this skill.",
		});

		expect(content).toContain('name: "true"');
		expect(parsePersonalSkillMarkdown(content)).toMatchObject({
			name: "true",
		});

		const numericNameContent = buildPersonalSkillMarkdown({
			name: "123",
			description: "",
			body: "Use this skill.",
		});
		expect(numericNameContent).toContain('name: "123"');
		expect(parsePersonalSkillMarkdown(numericNameContent)).toMatchObject({
			name: "123",
		});
	});

	it("escapes quoted description values", () => {
		const content = buildPersonalSkillMarkdown({
			name: "test-skill",
			description: 'Review "critical" C:\\paths.',
			body: "Use this skill.",
		});

		expect(content).toContain(
			'description: "Review \\"critical\\" C:\\\\paths."',
		);
		expect(parsePersonalSkillMarkdown(content)).toMatchObject({
			description: 'Review "critical" C:\\paths.',
		});
	});
});

describe("isValidPersonalSkillName", () => {
	it("accepts kebab-case skill names", () => {
		expect(isValidPersonalSkillName("test-skill-1")).toBe(true);
	});

	it("rejects names outside the backend pattern", () => {
		expect(isValidPersonalSkillName("Test Skill")).toBe(false);
		expect(isValidPersonalSkillName("test--skill")).toBe(false);
		expect(isValidPersonalSkillName("-test-skill")).toBe(false);
	});

	it("rejects names over the backend byte limit", () => {
		expect(isValidPersonalSkillName("a".repeat(256))).toBe(true);
		expect(isValidPersonalSkillName("a".repeat(257))).toBe(false);
	});
});

describe("isValidPersonalSkillDescription", () => {
	it("rejects descriptions over the backend byte limit", () => {
		expect(isValidPersonalSkillDescription("a".repeat(4096))).toBe(true);
		expect(isValidPersonalSkillDescription("a".repeat(4097))).toBe(false);
	});
});

describe("getPersonalSkillContentSizeBytes", () => {
	it("counts UTF-8 bytes", () => {
		expect(getPersonalSkillContentSizeBytes("a")).toBe(1);
		expect(getPersonalSkillContentSizeBytes("🧪")).toBe(4);
	});

	it("exposes the backend size limit", () => {
		expect(PERSONAL_SKILL_MAX_SIZE_BYTES).toBe(65_536);
	});
});
