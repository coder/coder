import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ComponentProps, useState } from "react";
import { expect, fn, userEvent, waitFor } from "storybook/test";
import { filterSkillsByQuery } from "../../utils/personalSkills";
import { COMPACT_SLASH_COMMAND } from "../../utils/slashCommands";
import {
	createCommandMenuItem,
	createSkillMenuItem,
	type SkillMetadata,
	SkillsTriggerMenu,
} from "./SkillsTriggerMenu";
import {
	expectInsideListViewport,
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

// Provides the composer-box element the menu anchors to, since the
// menu is pinned above its anchor at the anchor's width.
const MenuStoryHarness = (args: ComponentProps<typeof SkillsTriggerMenu>) => {
	const [anchor, setAnchor] = useState<HTMLDivElement | null>(null);
	return (
		<>
			<div
				ref={setAnchor}
				className="mt-64 h-16 w-64 rounded-md border border-border border-solid p-2 text-content-secondary text-sm"
			>
				Mock composer
			</div>
			<SkillsTriggerMenu {...args} anchor={anchor} />
		</>
	);
};

const meta: Meta<typeof SkillsTriggerMenu> = {
	title: "components/ChatMessageInput/SkillsTriggerMenu",
	component: SkillsTriggerMenu,
	args: {
		open: true,
		anchor: null,
		query: "",
		personalSkills: mockPersonalSkillItems,
		workspaceSkills: [],
		workspaceSkillsEnabled: false,
		onSelectedIndexChange: fn(),
		selectedIndex: 0,
		onSelect: fn(),
		onClose: fn(),
	},
	render: (args) => <MenuStoryHarness {...args} />,
	decorators: [
		(Story) => (
			<div className="h-96 p-6">
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

const manyPersonalSkillItems = Array.from({ length: 30 }, (_, index) =>
	createSkillMenuItem("personal", {
		name: `skill-${String(index).padStart(2, "0")}`,
		description: "",
	}),
);

// cmdk scrolls the controlled highlight into view only at mount, so the
// selection must move after mount to exercise the menu's own scrolling.
const SelectionScrollHarness = (
	args: ComponentProps<typeof SkillsTriggerMenu>,
) => {
	const [selectedIndex, setSelectedIndex] = useState(0);
	const [anchor, setAnchor] = useState<HTMLDivElement | null>(null);
	return (
		<>
			<button type="button" onClick={() => setSelectedIndex(29)}>
				Highlight last skill
			</button>
			<div ref={setAnchor} className="mt-96 h-16 w-96" />
			<SkillsTriggerMenu
				{...args}
				anchor={anchor}
				selectedIndex={selectedIndex}
				onSelectedIndexChange={setSelectedIndex}
			/>
		</>
	);
};

export const ScrollsSelectionIntoView: Story = {
	args: {
		personalSkills: manyPersonalSkillItems,
	},
	render: (args) => <SelectionScrollHarness {...args} />,
	play: async () => {
		await userEvent.click(await findVisibleText("Highlight last skill"));
		await expectInsideListViewport(await findVisibleText("/skill-29"));
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

// The menu opens above its anchor at the anchor's exact width,
// matching the mobile pinned-above-composer placement (CODAGT-956).
export const OpensAboveAnchorAtAnchorWidth: Story = {
	play: async () => {
		const item = await findVisibleText("/reviewer");
		const content = item.closest("[data-side]");
		expect(content).not.toBeNull();
		expect(content).toHaveAttribute("data-side", "top");
		const anchorBox = (await findVisibleText("Mock composer")).closest("div");
		expect(anchorBox).not.toBeNull();
		if (!anchorBox || !(content instanceof HTMLElement)) return;
		// The entrance animation scales the content from 95%, so wait
		// for the settled geometry.
		await waitFor(() => {
			const anchorRect = anchorBox.getBoundingClientRect();
			const contentRect = content.getBoundingClientRect();
			expect(contentRect.width).toBeCloseTo(anchorRect.width, 0);
			expect(contentRect.left).toBeCloseTo(anchorRect.left, 0);
			expect(contentRect.bottom).toBeLessThanOrEqual(anchorRect.top);
		});
	},
};
