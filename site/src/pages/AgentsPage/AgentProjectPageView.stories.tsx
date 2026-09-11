import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockChatProject } from "#/testHelpers/entities";
import { AgentProjectPageView } from "./AgentProjectPageView";

const meta = {
	title: "pages/AgentsPage/AgentProjectPageView",
	component: AgentProjectPageView,
	args: {
		chats: [],
		isLoading: false,
		onEdit: fn(),
		newChatPath: `/agents?project=${MockChatProject.id}`,
	},
} satisfies Meta<typeof AgentProjectPageView>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Loading: Story = {
	args: { isLoading: true },
};

export const Empty: Story = {
	args: { project: MockChatProject },
};

export const Populated: Story = {
	args: {
		project: MockChatProject,
		chats: [
			{ ...MockChat, id: "project-chat-1", title: "Prepare launch notes" },
			{ ...MockChat, id: "project-chat-2", title: "Review release checklist" },
		],
	},
};

export const Failed: Story = {
	args: {
		error: new globalThis.Error("Unable to load project."),
	},
};
