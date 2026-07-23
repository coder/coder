import { expect, waitFor, within } from "storybook/test";
import type { UserSkillMetadata } from "#/api/typesGenerated";
import { MOCK_TIMESTAMP } from "#/testHelpers/chatEntities";

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

// The 1px tolerance absorbs subpixel rounding in scrolled rects.
export const expectInsideListViewport = async (
	element: HTMLElement,
): Promise<void> => {
	const list = element.closest("[cmdk-list]");
	expect(list).not.toBeNull();
	await waitFor(() => {
		const listRect = (list as HTMLElement).getBoundingClientRect();
		const elementRect = element.getBoundingClientRect();
		expect(elementRect.top).toBeGreaterThanOrEqual(listRect.top - 1);
		expect(elementRect.bottom).toBeLessThanOrEqual(listRect.bottom + 1);
	});
};
