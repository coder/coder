import type { Meta, StoryObj } from "@storybook/react-vite";
import { type FC, useState } from "react";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { ChatSummaryPanel } from "./ChatSummaryPanel";

const ROOT_CHAT_ID = "root-chat-id";
const OTHER_CHAT_ID = "other-chat-id";

const LONG_SUMMARY = [
	"Audited the whole chat pipeline and shipped a batch of fixes.",
	"",
	...Array.from(
		{ length: 12 },
		(_, i) =>
			`- Reviewed subsystem number ${i + 1} and applied the corresponding fix so the behaviour matches the specification`,
	),
].join("\n");

const mockCost: TypesGen.ChatCost = {
	chat_id: MockChat.id,
	total_cost_micros: 1_250_000,
	request_count: 8,
	unpriced_request_count: 0,
};

type MockRequestOptions = {
	cost?: TypesGen.ChatCost;
	summary?: string | null;
	chatError?: boolean;
	parentChatId?: string;
	rootChatId?: string;
};

const mockRequests = ({
	cost = mockCost,
	summary = null,
	chatError,
	parentChatId,
	rootChatId,
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
			...(rootChatId ? { root_chat_id: rootChatId } : {}),
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
	parameters: { features: ["aibridge"] satisfies TypesGen.FeatureName[] },
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
			summary: [
				"Investigated the flaky CI job and landed a fix.",
				"",
				"- Traced it to a cache-layer race in `chatd.go`",
				"- Added a regression test covering the race",
			].join("\n"),
		}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.getByText(/Traced it to a cache-layer race/),
			).toBeInTheDocument();
			expect(canvas.getByText("$1.25")).toBeInTheDocument();
		});
		expect(
			within(canvas.getByRole("list")).getAllByRole("listitem"),
		).toHaveLength(2);
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

export const SubagentTreeCost: Story = {
	beforeEach: () =>
		mockRequests({
			parentChatId: "parent-chat-id",
			rootChatId: ROOT_CHAT_ID,
			cost: { ...mockCost, chat_id: ROOT_CHAT_ID },
		}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText("$1.25")).toBeInTheDocument();
		});
		expect(
			canvas.getByText(/Cost covers this agent's whole chat/),
		).toBeInTheDocument();
		expect(API.experimental.getChatCost).toHaveBeenCalledWith(ROOT_CHAT_ID);
		expect(API.experimental.getChatCost).not.toHaveBeenCalledWith(MockChat.id);
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

export const GatewayUnavailable: Story = {
	parameters: { features: [] },
	beforeEach: () => mockRequests({ summary: "Gateway is off here." }),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText("Gateway is off here.")).toBeInTheDocument();
		});
		expect(canvas.queryByText("Cost:")).not.toBeInTheDocument();
		expect(API.experimental.getChatCost).not.toHaveBeenCalled();
	},
};

// Navigating between chats swaps `chatId` on the existing panel instead of
// remounting it, so the disclosure state must not carry over. Without a reset
// the next chat renders fully expanded behind a "Show less" button, even when
// its own summary fits.
export const ExpansionResetsBetweenChats: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChat").mockImplementation(async (chatId) => ({
			...MockChat,
			id: chatId,
			summary: chatId === MockChat.id ? LONG_SUMMARY : "A summary that fits.",
		}));
		spyOn(API.experimental, "getChatCost").mockResolvedValue(mockCost);
	},
	render: (args) => {
		const [chatId, setChatId] = useState(MockChat.id);
		return (
			<div className="flex h-full min-h-0 flex-col">
				<button
					type="button"
					onClick={() =>
						setChatId((id) =>
							id === MockChat.id ? OTHER_CHAT_ID : MockChat.id,
						)
					}
				>
					Switch chat
				</button>
				<ChatSummaryPanel {...args} chatId={chatId} />
			</div>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const switchChat = canvas.getByRole("button", { name: "Switch chat" });
		const expand = async () => {
			const showMore = await canvas.findByRole("button", {
				name: "Show more",
			});
			await userEvent.click(showMore);
			await expect(
				canvas.getByRole("button", { name: "Show less" }),
			).toBeInTheDocument();
		};

		// Warm both chats in the query cache first. While a chat is still
		// uncached the panel renders nothing, which unmounts the summary and
		// resets the disclosure state as a side effect. Once cached, the data is
		// returned synchronously and the panel keeps the same summary instance
		// across the switch, which is where the state can leak.
		await expand();
		await userEvent.click(switchChat);
		await waitFor(async () => {
			await expect(canvas.getByText("A summary that fits.")).toBeVisible();
		});
		await userEvent.click(switchChat);

		await expand();
		await userEvent.click(switchChat);
		await waitFor(async () => {
			await expect(canvas.getByText("A summary that fits.")).toBeVisible();
		});

		// The switched-to summary fits, so it must not offer to collapse.
		await expect(
			canvas.queryByRole("button", { name: "Show less" }),
		).not.toBeInTheDocument();
	},
};
