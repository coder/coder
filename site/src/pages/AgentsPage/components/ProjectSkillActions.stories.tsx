import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent } from "storybook/test";
import { chatProjectSkillsKey } from "#/api/queries/chatProjectSkills";
import type { SkillMetadata } from "#/api/typesGenerated";
import { MockChatProject } from "#/testHelpers/entities";
import { MockSkill } from "#/testHelpers/skills";
import { ProjectSkillActions } from "./ProjectSkillActions";

const mockActionSkills: SkillMetadata[] = [
	{
		name: "deploy",
		description: "Deploy the current branch to the staging cluster.",
	},
	{ name: "lint", description: "Run the linters and fix what they report." },
	{
		name: "release-notes-for-the-next-version-with-every-merged-change",
		description: "Draft release notes for the next version.",
	},
	{ name: "review", description: "Review changed files and suggest fixes." },
	{ name: "triage", description: "" },
].map((skill) => ({
	...MockSkill,
	...skill,
	id: `skill-${skill.name}`,
	action: true,
}));

const meta: Meta<typeof ProjectSkillActions> = {
	title: "pages/AgentsPage/ProjectSkillActions",
	component: ProjectSkillActions,
	args: {
		organizationId: MockChatProject.organization_id,
		projectId: MockChatProject.id,
		onInvoke: fn(),
	},
	decorators: [
		(Story) => (
			<div className="w-120">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ProjectSkillActions>;

/** Hovers the action so the screenshot shows its description. */
export const OneAction: Story = {
	parameters: {
		queries: [
			{
				key: chatProjectSkillsKey(MockChatProject.id),
				data: mockActionSkills.slice(0, 1),
			},
		],
	},
	play: async () => {
		await userEvent.hover(
			await screen.findByRole("button", { name: "deploy" }),
		);
		await screen.findByRole("tooltip");
	},
};

export const ThreeActions: Story = {
	parameters: {
		queries: [
			{
				key: chatProjectSkillsKey(MockChatProject.id),
				data: mockActionSkills.slice(0, 3),
			},
		],
	},
};

/** Opens More so the screenshot shows the overflow actions. */
export const FiveActionsMoreOpen: Story = {
	parameters: {
		queries: [
			{ key: chatProjectSkillsKey(MockChatProject.id), data: mockActionSkills },
		],
	},
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", { name: "More project actions" }),
		);
	},
};

export const NarrowWidth: Story = {
	decorators: [
		(Story) => (
			<div className="w-72">
				<Story />
			</div>
		),
	],
	parameters: {
		queries: [
			{ key: chatProjectSkillsKey(MockChatProject.id), data: mockActionSkills },
		],
	},
};
