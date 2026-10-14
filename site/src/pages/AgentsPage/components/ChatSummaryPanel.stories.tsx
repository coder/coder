import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, spyOn, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat, MockChatMessage } from "#/testHelpers/chatEntities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { createChatStore } from "./ChatConversation/chatStore";
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
const PanelFrame = (Story: React.FC) => (
	<div className="h-[420px] w-[420px] max-w-full border border-solid border-border">
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
		store: createChatStore(),
		hasMoreMessages: false,
		isFetchingMoreMessages: false,
		hasFetchMoreError: false,
		onFetchMoreMessages: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ChatSummaryPanel>;

const artifactMessages: TypesGen.ChatMessage[] = [
	{
		...MockChatMessage,
		id: 1,
		role: "user",
		content: [
			{ type: "text", text: "Here is the failing log." },
			{
				type: "file",
				media_type: "text/plain",
				name: "uploaded-ci.log",
				file_id: "upload-file",
			},
		],
	},
	{
		...MockChatMessage,
		id: 2,
		role: "assistant",
		content: [
			{
				type: "tool-call",
				tool_call_id: "attach",
				tool_name: "attach_file",
				args: { path: "/home/coder/flaky-runs.csv" },
			},
			{
				type: "file",
				media_type: "text/csv",
				name: "flaky-runs.csv",
				file_id: "runs-file",
			},
		],
	},
	{
		...MockChatMessage,
		id: 3,
		role: "assistant",
		content: [
			{
				type: "tool-call",
				tool_call_id: "screenshot",
				tool_name: "computer",
				args: { action: "screenshot" },
			},
			{
				type: "file",
				media_type: "image/png",
				name: "ci-dashboard.png",
				file_id: "screenshot-file",
			},
		],
	},
];

const createArtifactStore = () => {
	const store = createChatStore();
	store.replaceMessages(artifactMessages);
	return store;
};

export const WithSummary: Story = {
	beforeEach: () =>
		mockRequests({
			summary:
				"Investigated the flaky CI job, traced it to a cache-layer race, and added a regression test.",
		}),
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
		expect(API.experimental.getChatCost).toHaveBeenCalledWith(
			ROOT_CHAT_ID,
			expect.any(AbortSignal),
		);
		expect(API.experimental.getChatCost).not.toHaveBeenCalledWith(
			MockChat.id,
			expect.any(AbortSignal),
		);
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

// The user's upload stays in the transcript and is not an artifact.
export const WithArtifacts: Story = {
	args: { store: createArtifactStore() },
	beforeEach: () =>
		mockRequests({
			summary:
				"Investigated the flaky CI job, traced it to a cache-layer race, and added a regression test.",
		}),
};
