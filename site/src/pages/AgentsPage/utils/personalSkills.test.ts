import { describe, expect, it } from "vitest";
import {
	buildPersonalSkillMarkdown,
	getPersonalSkillContentSizeBytes,
	isValidPersonalSkillName,
	PERSONAL_SKILL_MAX_SIZE_BYTES,
	parsePersonalSkillMarkdown,
	tryParsePersonalSkillMarkdown,
} from "./personalSkills";

describe("parsePersonalSkillMarkdown", () => {
	it("parses SKILL.md frontmatter and body", () => {
		expect(
			parsePersonalSkillMarkdown(
				'---\nname: test-skill\ndescription: "Does a thing"\n---\n\nUse this skill.',
			),
		).toEqual({
			name: "test-skill",
			description: "Does a thing",
			body: "Use this skill.",
		});
	});

	it("uses backend-compatible parsing for YAML-comment-sensitive values", () => {
		expect(
			parsePersonalSkillMarkdown(
				"---\nname: test-skill\ndescription: Build # test\n---\nBody",
			),
		).toEqual({
			name: "test-skill",
			description: "Build # test",
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

describe("getPersonalSkillContentSizeBytes", () => {
	it("counts UTF-8 bytes", () => {
		expect(getPersonalSkillContentSizeBytes("a")).toBe(1);
		expect(getPersonalSkillContentSizeBytes("🧪")).toBe(4);
	});

	it("exposes the backend size limit", () => {
		expect(PERSONAL_SKILL_MAX_SIZE_BYTES).toBe(65_536);
	});
});
