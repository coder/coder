import { flushSync } from "react-dom";
import { expect, userEvent, waitFor, within } from "storybook/test";
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

/**
 * Presses Escape with the listener ordering a real keypress produces.
 *
 * Browsers run a microtask checkpoint between listeners of a trusted
 * event, so React commits the state update from Radix's capture-phase
 * dismiss before Lexical's bubble-phase handler sees the same keydown.
 * Synthetic events from userEvent dispatch every listener on one stack,
 * hiding that ordering, so flush React explicitly in a capture-phase
 * listener registered after the popover's.
 */
export const pressEscapeAsBrowser = async (): Promise<void> => {
	const flushReact = () => flushSync(() => {});
	document.addEventListener("keydown", flushReact, true);
	try {
		await userEvent.keyboard("{Escape}");
	} finally {
		document.removeEventListener("keydown", flushReact, true);
	}
};

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
