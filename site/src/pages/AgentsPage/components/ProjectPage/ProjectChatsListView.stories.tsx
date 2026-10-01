import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockUserMember, mockApiError } from "#/testHelpers/entities";
import { ProjectChatsListView } from "./ProjectChatsListView";

const chats: Chat[] = [
	{
		...MockChat,
		id: "chat-running",
		title: "Add retries to the deploy script",
		status: "running",
		diff_status: {
			chat_id: "chat-running",
			url: "https://github.com/coder/coder/pull/1",
			pull_request_state: "open",
			pull_request_title: "Add retries to the deploy script",
			pull_request_draft: false,
			changes_requested: false,
			additions: 42,
			deletions: 7,
			changed_files: 3,
		},
	},
	{
		...MockChat,
		id: "chat-merged",
		title: "Fix the flaky login test",
		diff_status: {
			chat_id: "chat-merged",
			url: "https://github.com/coder/coder/pull/2",
			pull_request_state: "merged",
			pull_request_title: "Fix the flaky login test",
			pull_request_draft: false,
			changes_requested: false,
			additions: 3,
			deletions: 12,
			changed_files: 1,
		},
	},
	{
		...MockChat,
		id: "chat-other-owner",
		title: "Investigate the slow dashboard query",
		status: "error",
		owner_id: MockUserMember.id,
		owner_username: MockUserMember.username,
		owner_name: MockUserMember.name,
	},
	{
		...MockChat,
		id: "chat-long-title",
		title: "T".repeat(200),
	},
];

const meta: Meta<typeof ProjectChatsListView> = {
	title: "pages/AgentsPage/ProjectPage/ProjectChatsListView",
	component: ProjectChatsListView,
	decorators: [
		(Story) => (
			<div className="max-w-5xl p-6">
				<Story />
			</div>
		),
	],
	args: {
		chats,
		error: undefined,
		onRetry: fn(),
		hasNextPage: false,
		isFetchingNextPage: false,
		onLoadMore: fn(),
		currentUserId: MockChat.owner_id,
		actions: {
			requestArchiveAgent: fn(),
			requestUnarchiveAgent: fn(),
			requestArchiveAndDeleteWorkspace: fn(),
			requestPinAgent: fn(),
			requestUnpinAgent: fn(),
			onOpenRenameDialog: fn(),
			isArchiving: false,
		},
	},
};

export default meta;
type Story = StoryObj<typeof ProjectChatsListView>;

export const Populated: Story = {};

export const MoreToLoad: Story = {
	args: { hasNextPage: true, isFetchingNextPage: true },
};

export const RowActionsOpen: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", {
				name: `Open chat actions for ${chats[0].title}`,
			}),
		);
	},
};

export const WithoutActions: Story = {
	args: { actions: undefined },
};

export const Loading: Story = {
	args: { chats: undefined },
};

export const LoadError: Story = {
	args: {
		chats: undefined,
		error: mockApiError({ message: "Failed to load chats." }),
	},
};

export const Empty: Story = {
	args: { chats: [] },
};
