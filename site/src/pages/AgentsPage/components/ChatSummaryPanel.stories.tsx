import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { expect, spyOn, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { ChatSummaryPanel } from "./ChatSummaryPanel";

const mockCost: TypesGen.ChatCost = {
	chat_id: MockChat.id,
	total_cost_micros: 1_250_000,
	priced_message_count: 8,
	unpriced_messages_having_usage_count: 0,
};

type MockRequestOptions = {
	cost?: TypesGen.ChatCost;
	summary?: string | null;
	chatError?: boolean;
	parentChatId?: string;
};

const mockRequests = ({
	cost = mockCost,
	summary = null,
	chatError,
	parentChatId,
}: MockRequestOptions = {}) => {
	if (chatError) {
		spyOn(API.experimental, "getChat").mockRejectedValue(
			new Error("Failed to load chat"),
		);
	} else {
		spyOn(API.experimental, "getChat").mockResolvedValue({
			...MockChat,
			summary,
			...(parentChatId ? { parent_chat_id: parentChatId } : {}),
		});
	}

	spyOn(API.experimental, "getChatCost").mockResolvedValue(cost);
};

// The Summary tab fills the right panel, so give stories a bounded height.
const PanelFrame = (Story: FC) => (
	<div className="h-[420px] w-[420px] max-w-full border border-solid border-border-default">
		<Story />
	</div>
);

const meta: Meta<typeof ChatSummaryPanel> = {
	title: "pages/AgentsPage/ChatSummaryPanel",
	component: ChatSummaryPanel,
	decorators: [PanelFrame, withDashboardProvider],
	args: {
		chatId: MockChat.id,
		isVisible: true,
	},
};

export default meta;
type Story = StoryObj<typeof ChatSummaryPanel>;

export const WithSummary: Story = {
	beforeEach: () =>
		mockRequests({
			summary:
				"Investigated the flaky CI job, traced it to a cache-layer race, and added a regression test.",
		}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.getByText(/traced it to a cache-layer race/),
			).toBeInTheDocument();
			expect(canvas.getByText("$1.25")).toBeInTheDocument();
		});
	},
};

// A running subagent has no summary yet; its report is persisted as the
// summary when it completes, so the empty state reads as pending.
export const SubagentSummaryPending: Story = {
	beforeEach: () => mockRequests({ parentChatId: "parent-chat-id" }),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.getByText("Summary pending agent completion."),
			).toBeInTheDocument();
		});
	},
};

export const ChatError: Story = {
	beforeEach: () => mockRequests({ chatError: true }),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText("Failed to load chat")).toBeInTheDocument();
		});
	},
};

export const NotVisible: Story = {
	args: { isVisible: false },
	beforeEach: () => mockRequests({ summary: "Should never be fetched." }),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Gating disables both queries, so nothing renders and no API call fires.
		expect(API.experimental.getChat).not.toHaveBeenCalled();
		expect(API.experimental.getChatCost).not.toHaveBeenCalled();
		expect(
			canvas.queryByText("Should never be fetched."),
		).not.toBeInTheDocument();
		expect(canvas.queryByText("No summary yet.")).not.toBeInTheDocument();
	},
};
