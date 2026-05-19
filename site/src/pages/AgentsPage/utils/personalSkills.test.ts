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
});

describe("buildPersonalSkillMarkdown", () => {
	it("builds backend-compatible skill markdown", () => {
		const content = buildPersonalSkillMarkdown({
			name: "test-skill",
			description: "Does a thing",
			body: "Use this skill.",
		});

		expect(content).toBe(
			"---\nname: test-skill\ndescription: Does a thing\n---\nUse this skill.\n",
		);
		expect(parsePersonalSkillMarkdown(content)).toEqual({
			name: "test-skill",
			description: "Does a thing",
			body: "Use this skill.",
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
