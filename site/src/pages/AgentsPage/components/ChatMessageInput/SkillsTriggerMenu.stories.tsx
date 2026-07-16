import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { filterSkillsByQuery } from "../../utils/personalSkills";
import { createSkillMenuItem, SkillsTriggerMenu } from "./SkillsTriggerMenu";
import {
	expectNoVisibleText,
	findVisibleText,
	MockSkills,
} from "./storyHelpers";

const mockWorkspaceSkills: TypesGen.WorkspaceSkillMetadata[] = [
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

export const WorkspaceError: Story = {
	args: {
		personalSkills: [],
		workspaceSkills: [],
		workspaceSkillsEnabled: true,
		isWorkspaceError: true,
	},
	play: async () => {
		expect(
			await findVisibleText(
				"Could not load workspace skills. Close and type / again to retry.",
			),
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
