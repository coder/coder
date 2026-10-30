import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent } from "storybook/test";
import {
	MockChatProject,
	MockChatSecurityAgent,
	mockApiError,
} from "#/testHelpers/entities";
import { InvokableAgentsSectionView } from "./InvokableAgentsSection";

const mockDocsAgent = {
	...MockChatProject,
	id: "chat-project-docs",
	name: "Docs writer",
	icon: "/icon/github.svg",
};

const meta: Meta<typeof InvokableAgentsSectionView> = {
	title: "pages/AgentsPage/ProjectPage/InvokableAgentsSectionView",
	component: InvokableAgentsSectionView,
	decorators: [
		(Story) => (
			<div className="max-w-3xl">
				<Story />
			</div>
		),
	],
	args: {
		invokable: [mockDocsAgent],
		available: [MockChatSecurityAgent],
		agentsError: undefined,
		onRetryAgents: fn(),
		canEdit: true,
		isSaving: false,
		saveError: undefined,
		onAddAgent: fn(),
		onRemoveAgent: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof InvokableAgentsSectionView>;

export const Default: Story = {};

export const NoAgentsAdded: Story = { args: { invokable: [] } };

export const UnreadableAgent: Story = {
	args: { invokable: [{ id: "hidden", name: "" }] },
};

export const Saving: Story = { args: { isSaving: true } };

export const ReadOnly: Story = { args: { canEdit: false } };

export const AddMenuOpen: Story = {
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", { name: "Add agent" }),
		);
	},
};

export const NoOtherAgents: Story = {
	args: { available: [], invokable: [] },
	play: AddMenuOpen.play,
};

export const NothingLeftToAdd: Story = {
	args: { available: [] },
	play: AddMenuOpen.play,
};

export const SaveError: Story = {
	args: {
		saveError: mockApiError({
			message: "Invalid request.",
			validations: [
				{
					field: "invokable_agent_ids",
					detail:
						"You need edit access to agent Docs writer to let another agent invoke it.",
				},
			],
		}),
	},
};

export const AgentsLoadError: Story = {
	args: {
		agentsError: mockApiError({ message: "Failed to load agents." }),
	},
};

export const AddMenuNoMatch: Story = {
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", { name: "Add agent" }),
		);
		await userEvent.keyboard("zzz");
	},
};
