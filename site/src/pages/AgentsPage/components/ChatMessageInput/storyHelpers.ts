import { expect, waitFor, within } from "storybook/test";
import type { UserSkillMetadata } from "#/api/typesGenerated";

const MOCK_TIMESTAMP = "2026-05-08T00:00:00Z";

/**
 * Builds a UserSkillMetadata for stories. Defaults to a named skill with no
 * description; pass overrides for the fields a case cares about.
 */
export const makeUserSkill = (
	overrides: Partial<UserSkillMetadata> = {},
): UserSkillMetadata => ({
	id: "skill-1",
	name: "skill",
	description: "",
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
	...overrides,
});

export const mockSkills: UserSkillMetadata[] = [
	makeUserSkill({
		id: "skill-reviewer",
		name: "reviewer",
		description: "Review changed files and suggest fixes.",
	}),
	makeUserSkill({
		id: "skill-docs",
		name: "docs",
		description: "Draft docs for user-facing behavior.",
	}),
	makeUserSkill({ id: "skill-plan", name: "plan" }),
];

/**
 * Resolves once an element rendering `text` is actually visible. The skills
 * menu renders into a portal, so matches are searched on document.body and
 * filtered to laid-out elements rather than hidden duplicates.
 */
export const findVisibleText = async (text: string): Promise<HTMLElement> => {
	let visibleElement: HTMLElement | undefined;
	await waitFor(() => {
		const matches = within(document.body).queryAllByText(text);
		visibleElement = matches.find(
			(element) => element.getClientRects().length > 0,
		);
		expect(visibleElement).toBeDefined();
	});
	return visibleElement as HTMLElement;
};

export const expectNoVisibleText = async (text: string): Promise<void> => {
	await waitFor(() => {
		const matches = within(document.body).queryAllByText(text);
		expect(
			matches.every((element) => element.getClientRects().length === 0),
		).toBe(true);
	});
};
