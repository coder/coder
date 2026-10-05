import type { Meta, StoryObj } from "@storybook/react-vite";
import { chatProjectInstructionsKey } from "#/api/queries/chatProjects";
import { chatCostTreeKey, projectChatsKey } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { MockChat, mockChatCost } from "#/testHelpers/chatEntities";
import {
	MockChatProject,
	MockChatProjectInstructions,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { ProjectPage } from "./ProjectPage";

const pageQueries = (chats: Chat[]) => [
	{
		key: projectChatsKey(MockChatProject.id),
		data: { pages: [chats], pageParams: [0] },
	},
	{
		key: chatProjectInstructionsKey(MockChatProject.id),
		data: MockChatProjectInstructions,
	},
	...chats.map((chat) => ({
		key: chatCostTreeKey(chat.id),
		data: mockChatCost(chat.id, 1_234_000),
	})),
];

const meta: Meta<typeof ProjectPage> = {
	title: "pages/AgentsPage/ProjectPage/ProjectPage",
	component: ProjectPage,
	decorators: [
		(Story) => (
			<div className="flex h-screen flex-col">
				<Story />
			</div>
		),
		withAuthProvider,
		withDashboardProvider,
	],
	args: {
		project: MockChatProject,
		children: (
			<div className="flex h-32 items-center justify-center rounded-lg border border-dashed border-border text-sm text-content-secondary">
				Composer
			</div>
		),
	},
	parameters: {
		user: MockUserOwner,
		features: ["aibridge"],
		queries: pageQueries([
			{ ...MockChat, id: "chat-1", title: "Plan the launch" },
			{
				...MockChat,
				id: "chat-2",
				title: "Draft the announcement",
				status: "running",
			},
		]),
	},
};

export default meta;
type Story = StoryObj<typeof ProjectPage>;

export const Desktop: Story = {};

/** Enough chats to overflow a phone screen, so the page scrolls. */
export const Mobile: Story = {
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
		queries: pageQueries(
			Array.from({ length: 12 }, (_, index) => ({
				...MockChat,
				id: `chat-${index}`,
				title: `Chat ${index + 1}`,
			})),
		),
	},
};
