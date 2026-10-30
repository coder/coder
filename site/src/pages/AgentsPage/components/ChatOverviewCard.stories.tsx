import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import { chatCostTreeKey } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChat,
	MockChatDiffStatus,
	MockChatMessage,
} from "#/testHelpers/chatEntities";
import { mockApiError } from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { createChatStore } from "./ChatConversation/chatStore";
import { ChatOverviewCard } from "./ChatOverviewCard";

const mockCost: TypesGen.ChatCost = {
	chat_id: MockChat.id,
	total_cost_micros: 1_250_000,
	request_count: 8,
	unpriced_request_count: 0,
};

const createStore = () => {
	const store = createChatStore();
	store.replaceMessages([MockChatMessage]);
	return store;
};

const meta: Meta<typeof ChatOverviewCard> = {
	title: "pages/AgentsPage/ChatOverviewCard",
	component: ChatOverviewCard,
	decorators: [withDashboardProvider],
	parameters: {
		features: ["aibridge"] satisfies TypesGen.FeatureName[],
		queries: [{ key: chatCostTreeKey(MockChat.id), data: mockCost }],
	},
	args: {
		chat: { ...MockChat, diff_statuses: [MockChatDiffStatus] },
		repositories: new Map([
			[
				"/home/coder/coder",
				{
					repo_root: "/home/coder/coder",
					branch: "main",
					unified_diff: "+added\n-removed\n",
				},
			],
		]),
		hasMoreMessages: false,
		isHydratingMessages: false,
		onOpenTarget: fn(),
	},
	render: (args) => <ChatOverviewCard {...args} store={createStore()} />,
};

export default meta;
type Story = StoryObj<typeof ChatOverviewCard>;

export const Default: Story = {};

/** The earlier attachments fail to load, and Retry requests them again. */
export const EarlierAttachmentsError: Story = {
	args: { hasMoreMessages: true },
	beforeEach: () => {
		spyOn(API.experimental, "getChatAttachments")
			.mockRejectedValueOnce(
				mockApiError({ message: "Failed to get chat attachments." }),
			)
			.mockReturnValue(new Promise(() => {}));
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(await canvas.findByRole("button", { name: "Retry" }));
		await waitFor(() =>
			expect(API.experimental.getChatAttachments).toHaveBeenCalledTimes(2),
		);
	},
};
