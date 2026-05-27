import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { filterPersonalSkills } from "../../utils/personalSkills";
import { PersonalSkillsTriggerMenu } from "./PersonalSkillsTriggerMenu";

const now = "2026-05-08T00:00:00Z";

const mockSkills: TypesGen.UserSkillMetadata[] = [
	{
		id: "skill-reviewer",
		name: "reviewer",
		description: "Review changed files and suggest fixes.",
		created_at: now,
		updated_at: now,
	},
	{
		id: "skill-docs",
		name: "docs",
		description: "Draft docs for user-facing behavior.",
		created_at: now,
		updated_at: now,
	},
	{
		id: "skill-plan",
		name: "plan",
		description: "",
		created_at: now,
		updated_at: now,
	},
];

const findVisibleText = async (text: string) => {
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

const expectNoVisibleText = async (text: string) => {
	await waitFor(() => {
		const matches = within(document.body).queryAllByText(text);
		expect(
			matches.every((element) => element.getClientRects().length === 0),
		).toBe(true);
	});
};

const meta: Meta<typeof PersonalSkillsTriggerMenu> = {
	title: "components/ChatMessageInput/PersonalSkillsTriggerMenu",
	component: PersonalSkillsTriggerMenu,
	args: {
		open: true,
		anchorRect: { top: 120, left: 80, height: 20 },
		query: "",
		skills: mockSkills,
		onSelectedIndexChange: fn(),
		selectedIndex: 0,
		onSelect: fn(),
		onClose: fn(),
	},
	decorators: [
		(Story) => (
			<div className="h-80 p-6">
				<p className="text-content-secondary text-sm">
					The menu is anchored to a mock caret position.
				</p>
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof PersonalSkillsTriggerMenu>;

export const Open: Story = {
	play: async () => {
		expect(await findVisibleText("/reviewer")).toBeDefined();
		expect(
			await findVisibleText("Review changed files and suggest fixes."),
		).toBeDefined();
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
		skills: [],
	},
	play: async () => {
		expect(await findVisibleText("Loading personal skills...")).toBeDefined();
	},
};

export const ErrorState: Story = {
	args: {
		isError: true,
		skills: [],
	},
	play: async () => {
		expect(
			await findVisibleText(
				"Could not load personal skills. Close and type / again to retry.",
			),
		).toBeDefined();
	},
};

export const Empty: Story = {
	args: {
		skills: [],
	},
	play: async () => {
		expect(await findVisibleText("No personal skills found.")).toBeDefined();
	},
};

export const FilteredEmpty: Story = {
	args: {
		query: "xyz",
		skills: [],
	},
	play: async () => {
		expect(
			await findVisibleText("No personal skills match that query."),
		).toBeDefined();
	},
};

export const Filtered: Story = {
	args: {
		query: "rev",
		skills: filterPersonalSkills(mockSkills, "rev"),
	},
	play: async () => {
		expect(await findVisibleText("/reviewer")).toBeDefined();
		await expectNoVisibleText("/docs");
	},
};

export const SelectsByClick: Story = {
	args: {
		onSelect: fn(),
	},
	play: async ({ args }) => {
		await userEvent.click(await findVisibleText("/reviewer"));
		expect(args.onSelect).toHaveBeenCalledTimes(1);
		expect(args.onSelect).toHaveBeenCalledWith(mockSkills[0]);
	},
};
