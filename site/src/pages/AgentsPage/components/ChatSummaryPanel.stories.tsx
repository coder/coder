import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { expect, spyOn, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { ChatSummaryPanel } from "./ChatSummaryPanel";

const chatId = "chat-1";

const mockChat: TypesGen.Chat = {
	id: chatId,
	organization_id: "org-1",
	owner_id: "owner-1",
	last_model_config_id: "model-1",
	title: "Test chat",
	status: "waiting",
	last_turn_summary: null,
	summary: null,
	created_at: "2024-05-01T12:00:00Z",
	updated_at: "2024-05-02T15:30:00Z",
	archived: false,
	shared: false,
	pin_order: 0,
	mcp_server_ids: [],
	labels: {},
	has_unread: false,
	client_type: "ui",
	children: [],
};

const mockCost: TypesGen.ChatCost = {
	root_chat_id: chatId,
	total_cost_micros: 1_250_000,
	priced_message_count: 8,
	unpriced_message_count: 0,
};

type MockRequestOptions = {
	cost?: TypesGen.ChatCost;
	costPending?: boolean;
	costError?: boolean;
	summary?: string | null;
	chatError?: boolean;
};

const mockRequests = ({
	cost = mockCost,
	costPending,
	costError,
	summary = null,
	chatError,
}: MockRequestOptions = {}) => {
	if (chatError) {
		spyOn(API.experimental, "getChat").mockRejectedValue(
			new Error("Failed to load chat"),
		);
	} else {
		spyOn(API.experimental, "getChat").mockResolvedValue({
			...mockChat,
			summary,
		});
	}

	if (costPending) {
		spyOn(API.experimental, "getChatCost").mockReturnValue(
			new Promise<TypesGen.ChatCost>(() => undefined),
		);
	} else if (costError) {
		spyOn(API.experimental, "getChatCost").mockRejectedValue(
			new Error("Failed to load cost"),
		);
	} else {
		spyOn(API.experimental, "getChatCost").mockResolvedValue(cost);
	}
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
		chatId,
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
		expect(canvas.queryByText("No summary yet.")).not.toBeInTheDocument();
	},
};

export const NoSummary: Story = {
	beforeEach: () => mockRequests(),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText("No summary yet.")).toBeInTheDocument();
			expect(canvas.getByText("$1.25")).toBeInTheDocument();
		});
	},
};

export const CostLoading: Story = {
	beforeEach: () => mockRequests({ costPending: true }),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByLabelText("Loading cost")).toBeInTheDocument();
		});
	},
};

export const CostError: Story = {
	beforeEach: () => mockRequests({ costError: true }),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText("Unavailable")).toBeInTheDocument();
		});
	},
};

export const PartialCost: Story = {
	beforeEach: () =>
		mockRequests({
			cost: {
				root_chat_id: chatId,
				total_cost_micros: 0,
				priced_message_count: 0,
				unpriced_message_count: 3,
			},
		}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.getByText(/Excludes 3 messages without model pricing\./),
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
