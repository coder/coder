import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent } from "storybook/test";
import { builtInSlashCommands } from "../../utils/builtInSlashCommands";
import { filterPersonalSkills } from "../../utils/personalSkills";
import { PersonalSkillsTriggerMenu } from "./PersonalSkillsTriggerMenu";
import {
	expectNoVisibleText,
	findVisibleText,
	MockSkills,
} from "./storyHelpers";

const meta: Meta<typeof PersonalSkillsTriggerMenu> = {
	title: "components/ChatMessageInput/PersonalSkillsTriggerMenu",
	component: PersonalSkillsTriggerMenu,
	args: {
		open: true,
		anchorRect: { top: 120, left: 80, height: 20 },
		query: "",
		builtInCommands: builtInSlashCommands,
		skills: MockSkills,
		onSelectedIndexChange: fn(),
		selectedIndex: 0,
		onCommandSelect: fn(),
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
		expect(await findVisibleText("/compact")).toBeDefined();
		expect(await findVisibleText("/reviewer")).toBeDefined();
		expect(
			await findVisibleText("Review changed files and suggest fixes."),
		).toBeDefined();
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
		builtInCommands: [],
		skills: [],
	},
	play: async () => {
		expect(await findVisibleText("Loading personal skills...")).toBeDefined();
	},
};

export const ErrorState: Story = {
	args: {
		isError: true,
		builtInCommands: [],
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
		builtInCommands: [],
		skills: [],
	},
	play: async () => {
		expect(
			await findVisibleText("No slash commands or personal skills found."),
		).toBeDefined();
	},
};

export const FilteredEmpty: Story = {
	args: {
		query: "xyz",
		builtInCommands: [],
		skills: [],
	},
	play: async () => {
		expect(
			await findVisibleText(
				"No slash commands or personal skills match that query.",
			),
		).toBeDefined();
	},
};

export const Filtered: Story = {
	args: {
		query: "rev",
		builtInCommands: [],
		skills: filterPersonalSkills(MockSkills, "rev"),
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
		expect(args.onSelect).toHaveBeenCalledWith(MockSkills[0]);
	},
};

export const SelectsCompactByClick: Story = {
	args: {
		onCommandSelect: fn(),
		onSelect: fn(),
	},
	play: async ({ args }) => {
		await userEvent.click(await findVisibleText("/compact"));
		expect(args.onCommandSelect).toHaveBeenCalledTimes(1);
		expect(args.onCommandSelect).toHaveBeenCalledWith(builtInSlashCommands[0]);
		expect(args.onSelect).not.toHaveBeenCalled();
	},
};
