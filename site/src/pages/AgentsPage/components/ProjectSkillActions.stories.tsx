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
	{ name: "release", description: "Draft release notes for the next version." },
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
			<div className="w-[480px]">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ProjectSkillActions>;

export const OneAction: Story = {
	parameters: {
		queries: [
			{
				key: chatProjectSkillsKey(MockChatProject.id),
				data: mockActionSkills.slice(0, 1),
			},
		],
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

/** Actions past the third move to More, opened here. */
export const FiveActionsMoreOpen: Story = {
	parameters: {
		queries: [
			{ key: chatProjectSkillsKey(MockChatProject.id), data: mockActionSkills },
		],
	},
	play: async () => {
		await userEvent.click(await screen.findByRole("button", { name: "More" }));
	},
};
