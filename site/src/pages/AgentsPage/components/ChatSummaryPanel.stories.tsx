import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChat,
	MockChatContextClean,
	MockChatContextDirty,
} from "#/testHelpers/chatEntities";
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
		contextUsage: {
			usedTokens: 68_000,
			contextLimitTokens: 100_000,
			compressionThreshold: 70,
		},
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
};

export const WithAgentResources: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChat").mockResolvedValue({
			...MockChat,
			summary: "Updated the agent resource details in the Summary panel.",
			context: MockChatContextClean,
		});
		spyOn(API.experimental, "getChatCost").mockResolvedValue(mockCost);
	},
};

export const ExpandedAgentResources: Story = {
	beforeEach: WithAgentResources.beforeEach,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: /^Context/ }),
		);
		await userEvent.click(canvas.getByRole("button", { name: /^Skills/ }));
		await userEvent.click(canvas.getByRole("button", { name: /^MCP servers/ }));
	},
};

export const EmptyResourcePlaceholders: Story = {
	args: { contextUsage: null },
	beforeEach: () =>
		mockRequests({ summary: "No workspace resources were found." }),
};

export const ResourceWarningCollapsed: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChat").mockResolvedValue({
			...MockChat,
			summary: "Some agent resources need attention.",
			context: MockChatContextDirty,
		});
		spyOn(API.experimental, "getChatCost").mockResolvedValue(mockCost);
	},
};

// A running subagent has no summary yet; its report is persisted as the
// summary when it completes, so the empty state reads as pending.
export const SubagentSummaryPending: Story = {
	beforeEach: () => mockRequests({ parentChatId: "parent-chat-id" }),
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
