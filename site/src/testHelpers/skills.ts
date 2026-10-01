import type { UserSkillMetadata } from "#/api/typesGenerated";
import { MOCK_TIMESTAMP } from "./chatEntities";

export const MockSkill: UserSkillMetadata = {
	id: "skill-1",
	name: "skill",
	description: "",
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

export const MockSkills: UserSkillMetadata[] = [
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
