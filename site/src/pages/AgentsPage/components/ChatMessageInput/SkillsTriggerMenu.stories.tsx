import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { filterSkillsByQuery } from "../../utils/personalSkills";
import { createSkillMenuItem, SkillsTriggerMenu } from "./SkillsTriggerMenu";

const now = "2026-05-08T00:00:00Z";

const mockPersonalSkills: TypesGen.UserSkillMetadata[] = [
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

const mockPersonalSkillItems = mockPersonalSkills.map((skill) =>
	createSkillMenuItem("personal", skill, false),
);
const mockWorkspaceSkillItems = mockWorkspaceSkills.map((skill) =>
	createSkillMenuItem("workspace", skill, false),
);

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

export const WorkspaceOnly: Story = {
	args: {
		personalSkills: [],
		workspaceSkills: mockWorkspaceSkillItems,
		workspaceSkillsEnabled: true,
	},
	play: async () => {
		await expectNoVisibleText("Personal skills");
		expect(await findVisibleText("Workspace skills")).toBeDefined();
		expect(await findVisibleText("/test-runner")).toBeDefined();
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
		expect(await findVisibleText("/test-runner")).toBeDefined();
	},
};

export const Collision: Story = {
	args: {
		personalSkills: [
			createSkillMenuItem("personal", mockPersonalSkills[0], true),
		],
		workspaceSkills: [
			createSkillMenuItem(
				"workspace",
				{
					name: "reviewer",
					description: "Workspace-specific review process.",
				},
				true,
			),
		],
		workspaceSkillsEnabled: true,
	},
	play: async () => {
		expect(await findVisibleText("/personal/reviewer")).toBeDefined();
		expect(await findVisibleText("/workspace/reviewer")).toBeDefined();
		expect(
			await findVisibleText("Workspace-specific review process."),
		).toBeDefined();
	},
};

export const EmptyWorkspaceWithPersonal: Story = {
	args: {
		workspaceSkills: [],
		workspaceSkillsEnabled: true,
	},
	play: async () => {
		expect(await findVisibleText("Personal skills")).toBeDefined();
		await expectNoVisibleText("Workspace skills");
	},
};

export const WorkspaceLoading: Story = {
	args: {
		workspaceSkills: [],
		workspaceSkillsEnabled: true,
		isWorkspaceLoading: true,
	},
	play: async () => {
		expect(await findVisibleText("Personal skills")).toBeDefined();
		expect(await findVisibleText("Loading workspace skills...")).toBeDefined();
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
		await expectNoVisibleText("/test-runner");
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
