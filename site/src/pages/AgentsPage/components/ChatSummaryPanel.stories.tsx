import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { expect, spyOn, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { ChatSummaryPanel } from "./ChatSummaryPanel";

const ROOT_CHAT_ID = "root-chat-id";

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
	status?: TypesGen.ChatStatus;
};

const mockRequests = ({
	cost = mockCost,
	summary = null,
	chatError,
	parentChatId,
	rootChatId,
	status,
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
			...(status ? { status } : {}),
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

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChat").mockImplementation(
			() => new Promise<TypesGen.Chat>(() => {}),
		);
		spyOn(API.experimental, "getChatCost").mockResolvedValue(mockCost);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByLabelText("Loading summary")).toBeVisible();
		expect(API.experimental.getChatCost).not.toHaveBeenCalled();
	},
};

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
		expect(canvas.queryByText("Generating summary")).not.toBeInTheDocument();
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
		expect(
			canvas.queryByText("Not enough details to summarize."),
		).not.toBeInTheDocument();
	},
};

export const NoSummary: Story = {
	beforeEach: () => mockRequests(),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.getByText("Not enough details to summarize."),
			).toBeInTheDocument();
		});
		expect(
			canvas.getByText(
				"A recap of your chat will appear here after a few more messages.",
			),
		).toBeInTheDocument();
		expect(canvas.queryByText("Generating summary")).not.toBeInTheDocument();
		expect(canvas.getByText("Created:")).toBeInTheDocument();
		expect(canvas.getByText("Updated:")).toBeInTheDocument();
		expect(canvas.getByText("Cost:")).toBeInTheDocument();
		expect(canvas.getByText("$1.25")).toBeInTheDocument();
	},
};

export const GeneratingSummary: Story = {
	args: { isGenerating: true },
	beforeEach: () => mockRequests({ status: "waiting" }),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const status = await canvas.findByRole("status");
		expect(status).toHaveTextContent("Generating summary");
		expect(
			canvas.queryByText("Not enough details to summarize."),
		).not.toBeInTheDocument();
		expect(canvas.getByText("Created:")).toBeInTheDocument();
		expect(canvas.getByText("Cost:")).toBeInTheDocument();
	},
};

// A short completed chat (for example, a single "test" prompt) is waiting but
// never emits chat_summary_generating, so it must keep the empty state.
export const ShortChatDoesNotGenerateSummary: Story = {
	beforeEach: () => mockRequests({ status: "waiting" }),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.getByText("Not enough details to summarize."),
			).toBeInTheDocument();
		});
		expect(canvas.queryByText("Generating summary")).not.toBeInTheDocument();
		expect(canvas.getByText("Cost:")).toBeInTheDocument();
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
