import type { SkillMetadata } from "#/api/typesGenerated";
import { MOCK_TIMESTAMP } from "./chatEntities";

export const MockSkill: SkillMetadata = {
	id: "skill-1",
	name: "skill",
	description: "",
	enabled: true,
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

export const MockSkills: SkillMetadata[] = [
	{
		...MockSkill,
		id: "skill-reviewer",
		name: "reviewer",
		description: "Review changed files and suggest fixes.",
	},
	{
		...MockSkill,
		id: "skill-docs",
		name: "docs",
		description: "Draft docs for user-facing behavior.",
	},
	{ ...MockSkill, id: "skill-plan", name: "plan" },
];

export const MockDisabledSkill: SkillMetadata = {
	...MockSkill,
	id: "skill-legacy-review",
	name: "legacy-review",
	description: "Older review checklist kept for reference.",
	enabled: false,
};
