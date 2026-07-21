import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent } from "storybook/test";
import { filterSkillsByQuery } from "../../utils/personalSkills";
import { COMPACT_SLASH_COMMAND } from "../../utils/slashCommands";
import {
	createCommandMenuItem,
	createSkillMenuItem,
	type SkillMetadata,
	SkillsTriggerMenu,
} from "./SkillsTriggerMenu";
import {
	expectNoVisibleText,
	findVisibleText,
	MockSkills,
} from "./storyHelpers";

const mockWorkspaceSkills: SkillMetadata[] = [
	{
		name: "test-runner",
		description: "Run the workspace test command.",
	},
	{
		name: "workspace-docs",
		description: "Use repository documentation conventions.",
	},
];

const mockPersonalSkillItems = MockSkills.map((skill) =>
	createSkillMenuItem("personal", skill),
);
const compactCommandItem = createCommandMenuItem(COMPACT_SLASH_COMMAND);
const mockWorkspaceSkillItems = mockWorkspaceSkills.map((skill) =>
	createSkillMenuItem("workspace", skill),
);

const meta: Meta<typeof SkillsTriggerMenu> = {
	title: "components/ChatMessageInput/SkillsTriggerMenu",
	component: SkillsTriggerMenu,
	args: {
		open: true,
		anchorRect: { top: 120, left: 80, height: 20 },
		query: "",
		personalSkills: mockPersonalSkillItems,
		workspaceSkills: [],
		workspaceSkillsEnabled: false,
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
type Story = StoryObj<typeof SkillsTriggerMenu>;

export const PersonalOnly: Story = {
	play: async () => {
		expect(await findVisibleText("Personal skills")).toBeDefined();
		expect(await findVisibleText("/reviewer")).toBeDefined();
		expect(
			await findVisibleText("Review changed files and suggest fixes."),
		).toBeDefined();
		await expectNoVisibleText("Workspace skills");
	},
};

export const BothGroups: Story = {
	args: {
		workspaceSkills: mockWorkspaceSkillItems,
		workspaceSkillsEnabled: true,
	},
	play: async () => {
		expect(await findVisibleText("Personal skills")).toBeDefined();
		expect(await findVisibleText("Workspace skills")).toBeDefined();
		expect(await findVisibleText("/reviewer")).toBeDefined();
		expect(await findVisibleText("/workspace/test-runner")).toBeDefined();
	},
};

export const Loading: Story = {
	args: {
		isPersonalLoading: true,
		personalSkills: [],
	},
	play: async () => {
		expect(await findVisibleText("Loading personal skills...")).toBeDefined();
	},
};

export const WorkspaceLoading: Story = {
	args: {
		personalSkills: [],
		workspaceSkills: [],
		workspaceSkillsEnabled: true,
		isWorkspaceLoading: true,
	},
	play: async () => {
		expect(await findVisibleText("Loading workspace skills...")).toBeDefined();
	},
};

export const EmptyWithWorkspace: Story = {
	args: {
		personalSkills: [],
		workspaceSkills: [],
		workspaceSkillsEnabled: true,
	},
	play: async () => {
		expect(
			await findVisibleText("No personal or workspace skills found."),
		).toBeDefined();
	},
};

export const Empty: Story = {
	args: {
		personalSkills: [],
		workspaceSkills: [],
	},
	play: async () => {
		expect(await findVisibleText("No personal skills found.")).toBeDefined();
	},
};

export const Filtered: Story = {
	args: {
		query: "rev",
		personalSkills: filterSkillsByQuery(mockPersonalSkillItems, "rev"),
		workspaceSkills: filterSkillsByQuery(mockWorkspaceSkillItems, "rev"),
		workspaceSkillsEnabled: true,
	},
	play: async () => {
		expect(await findVisibleText("/reviewer")).toBeDefined();
		await expectNoVisibleText("/docs");
		await expectNoVisibleText("/workspace/test-runner");
	},
};

export const SelectsByClick: Story = {
	args: {
		onSelect: fn(),
	},
	play: async ({ args }) => {
		await userEvent.click(await findVisibleText("/reviewer"));
		expect(args.onSelect).toHaveBeenCalledTimes(1);
		expect(args.onSelect).toHaveBeenCalledWith(mockPersonalSkillItems[0]);
	},
};

// Built-in commands render in a separate "Commands" group above
// personal skills and stay selectable alongside them.
export const WithCommands: Story = {
	args: {
		commands: [compactCommandItem],
	},
	play: async () => {
		expect(await findVisibleText("Commands")).toBeDefined();
		expect(await findVisibleText("/compact")).toBeDefined();
		expect(await findVisibleText("Personal skills")).toBeDefined();
		expect(await findVisibleText("/reviewer")).toBeDefined();
	},
};

// With no skills configured, the menu still opens to offer the
// built-in commands without any skills group or empty message.
export const CommandsOnly: Story = {
	args: {
		commands: [compactCommandItem],
		personalSkills: [],
	},
	play: async () => {
		expect(await findVisibleText("/compact")).toBeDefined();
		expect(
			await findVisibleText(
				"Summarize the conversation so far to free up context window space",
			),
		).toBeDefined();
		await expectNoVisibleText("No personal skills found.");
	},
};

export const SelectsCommandByClick: Story = {
	args: {
		commands: [compactCommandItem],
		onSelect: fn(),
	},
	play: async ({ args }) => {
		await userEvent.click(await findVisibleText("/compact"));
		expect(args.onSelect).toHaveBeenCalledTimes(1);
		expect(args.onSelect).toHaveBeenCalledWith(compactCommandItem);
	},
};
