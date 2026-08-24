import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { expect, spyOn, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
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

export const WithPreviewLinks: Story = {
	args: {
		workspace: MockWorkspace,
		workspaceAgent: MockWorkspaceAgent,
		wildcardHostname: "*.proxy.example.com",
	},
	beforeEach: () => {
		mockRequests({ summary: "Built the storybook and started a dev server." });
		spyOn(API, "getAgentListeningPorts").mockResolvedValue({
			ports: [
				{ process_name: "node", network: "tcp", port: 8080 },
				{ process_name: "node", network: "tcp", port: 6006 },
			],
		});
		spyOn(API, "getWorkspaceAgentSharedPorts").mockResolvedValue({
			shares: [],
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText("Preview:")).toBeInTheDocument();
		});
		const storybookLink = canvas.getByRole("link", {
			name: /Storybook \(6006\)/,
		});
		expect(storybookLink).toHaveAttribute(
			"href",
			"http://6006--a-workspace-agent--test-workspace--testuser.proxy.example.com/",
		);
		expect(
			canvas.getByRole("link", { name: /Preview \(8080\)/ }),
		).toBeInTheDocument();
	},
};

export const NoPreviewWithoutWildcardHost: Story = {
	args: {
		workspace: MockWorkspace,
		workspaceAgent: MockWorkspaceAgent,
		wildcardHostname: "",
	},
	beforeEach: () => {
		mockRequests({ summary: "No wildcard access URL configured." });
		spyOn(API, "getAgentListeningPorts");
		spyOn(API, "getWorkspaceAgentSharedPorts");
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.getByText("No wildcard access URL configured."),
			).toBeInTheDocument();
		});
		expect(canvas.queryByText("Preview:")).not.toBeInTheDocument();
		expect(API.getAgentListeningPorts).not.toHaveBeenCalled();
		expect(API.getWorkspaceAgentSharedPorts).not.toHaveBeenCalled();
	},
};
