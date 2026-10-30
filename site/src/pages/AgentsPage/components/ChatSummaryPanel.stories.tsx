import type { Meta, StoryObj } from "@storybook/react-vite";
import { onlineManager } from "react-query";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import {
	chatAttachmentsKey,
	chatCostTreeKey,
	chatEntityKey,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChat,
	MockChatAttachment,
	MockChatFileMetadata,
	MockChatMessage,
} from "#/testHelpers/chatEntities";
import { mockApiError } from "#/testHelpers/entities";
import { MockPngBase64 } from "#/testHelpers/files";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { createChatStore } from "./ChatConversation/chatStore";
import { attachmentStepMessages } from "./ChatConversation/storyFixtures";
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
		hasMoreMessages: false,
	},
	render: (args) => <ChatSummaryPanel {...args} store={createChatStore()} />,
};

export default meta;
type Story = StoryObj<typeof ChatSummaryPanel>;

const mockUploadPart: TypesGen.ChatFilePart = {
	type: "file",
	media_type: "text/plain",
	name: "uploaded-ci.log",
	file_id: "upload-file",
};

const mockScreenshotPart: TypesGen.ChatFilePart = {
	type: "file",
	media_type: "image/png",
	name: "ci-dashboard.png",
	data: MockPngBase64,
};

const mockAttachmentMessages: TypesGen.ChatMessage[] = [
	{
		...MockChatMessage,
		id: 1,
		role: "user",
		content: [
			{ type: "text", text: "Here is the failing log." },
			mockUploadPart,
		],
	},
	...attachmentStepMessages({
		id: 2,
		toolCallId: "attach",
		callAt: 1,
		toolName: "attach_file",
		args: { path: "/home/coder/flaky-runs.csv" },
		file: {
			type: "file",
			media_type: "text/csv",
			name: "flaky-runs.csv",
			file_id: "runs-file",
		},
		result: { name: "flaky-runs.csv" },
	}),
	...attachmentStepMessages({
		id: 5,
		toolCallId: "screenshot",
		callAt: 3,
		toolName: "computer",
		args: { action: "screenshot" },
		file: mockScreenshotPart,
		result: { data: "", mime_type: "image/png" },
	}),
];

const createAttachmentStore = (messages = mockAttachmentMessages) => {
	const store = createChatStore();
	store.upsertDurableMessages(messages, 1);
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

const mockSummaryChat: TypesGen.Chat = {
	...MockChat,
	summary:
		"Investigated the flaky CI job, traced it to a cache-layer race, and added a regression test.",
};

export const WithAttachments: Story = {
	render: (args) => (
		<ChatSummaryPanel {...args} store={createAttachmentStore()} />
	),
	parameters: {
		queries: [
			{ key: chatEntityKey(MockChat.id), data: mockSummaryChat },
			{ key: chatCostTreeKey(MockChat.id), data: mockCost },
		],
	},
};

const mockLatestMessages: TypesGen.ChatMessage[] = attachmentStepMessages({
	id: 20,
	toolCallId: "report",
	callAt: 10,
	toolName: "attach_file",
	args: { path: "/home/coder/latency-report.csv" },
	file: {
		type: "file",
		media_type: "text/csv",
		name: "latency-report.csv",
		file_id: "report-file",
	},
	result: { name: "latency-report.csv" },
});

const renderWithLatestMessages: Story["render"] = (args) => (
	<ChatSummaryPanel
		{...args}
		store={createAttachmentStore(mockLatestMessages)}
	/>
);

const mockEarlierScreenshot: TypesGen.ChatAttachment = {
	...MockChatAttachment,
	message_id: 6,
	role: "assistant",
	file: mockScreenshotPart,
};

const mockEarlierUpload: TypesGen.ChatAttachment = {
	...MockChatAttachment,
	message_id: 1,
	part_index: 1,
	file: mockUploadPart,
};

export const WithEarlierAttachments: Story = {
	args: { hasMoreMessages: true },
	render: renderWithLatestMessages,
	parameters: {
		queries: [
			{ key: chatEntityKey(MockChat.id), data: mockSummaryChat },
			{ key: chatCostTreeKey(MockChat.id), data: mockCost },
			{
				key: chatAttachmentsKey(MockChat.id, mockLatestMessages[0].id),
				data: { attachments: [mockEarlierScreenshot, mockEarlierUpload] },
			},
		],
	},
};

export const EarlierAttachmentsLoading: Story = {
	args: { hasMoreMessages: true },
	beforeEach: () => {
		spyOn(API.experimental, "getChatAttachments").mockReturnValue(
			new Promise(() => {}),
		);
	},
	render: renderWithLatestMessages,
	parameters: {
		queries: [
			{ key: chatEntityKey(MockChat.id), data: mockSummaryChat },
			{ key: chatCostTreeKey(MockChat.id), data: mockCost },
		],
	},
};

const mockAttachmentsError = mockApiError({
	message: "Failed to get chat attachments.",
});

export const EarlierAttachmentsFailed: Story = {
	args: { hasMoreMessages: true },
	beforeEach: () => {
		spyOn(API.experimental, "getChatAttachments").mockRejectedValue(
			mockAttachmentsError,
		);
	},
	render: renderWithLatestMessages,
	parameters: {
		queries: [
			{ key: chatEntityKey(MockChat.id), data: mockSummaryChat },
			{ key: chatCostTreeKey(MockChat.id), data: mockCost },
		],
	},
	// The alert pushes Retry below the panel frame's fold.
	play: async ({ canvasElement }) => {
		const retry = await within(canvasElement).findByRole("button", {
			name: "Retry",
		});
		retry.scrollIntoView();
	},
};

export const EarlierAttachmentsRetrying: Story = {
	args: { hasMoreMessages: true },
	beforeEach: () => {
		spyOn(API.experimental, "getChatAttachments")
			.mockRejectedValueOnce(mockAttachmentsError)
			.mockReturnValue(new Promise(() => {}));
	},
	render: renderWithLatestMessages,
	parameters: {
		queries: [
			{ key: chatEntityKey(MockChat.id), data: mockSummaryChat },
			{ key: chatCostTreeKey(MockChat.id), data: mockCost },
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("button", { name: "Retry" }),
		);
	},
};

// A Retry clicked offline pauses until the network returns, so the screenshot
// shows the alert with Retry enabled.
export const EarlierAttachmentsRetryingOffline: Story = {
	args: { hasMoreMessages: true },
	beforeEach: () => {
		spyOn(API.experimental, "getChatAttachments").mockRejectedValue(
			mockAttachmentsError,
		);
		return () => onlineManager.setOnline(true);
	},
	render: renderWithLatestMessages,
	parameters: {
		queries: [
			{ key: chatEntityKey(MockChat.id), data: mockSummaryChat },
			{ key: chatCostTreeKey(MockChat.id), data: mockCost },
		],
	},
	play: async ({ canvasElement }) => {
		const retry = await within(canvasElement).findByRole("button", {
			name: "Retry",
		});
		onlineManager.setOnline(false);
		await userEvent.click(retry);
	},
};

// After the Retry fails too, a reconnect refetches the attachments without a
// Retry, so the screenshot shows the loading state.
export const EarlierAttachmentsRefetchAfterFailedRetry: Story = {
	args: { hasMoreMessages: true },
	beforeEach: () => {
		spyOn(API.experimental, "getChatAttachments")
			.mockRejectedValueOnce(mockAttachmentsError)
			.mockRejectedValueOnce(
				mockApiError({
					message: "Failed to get chat attachments.",
					detail: "context deadline exceeded",
				}),
			)
			.mockReturnValue(new Promise(() => {}));
	},
	render: renderWithLatestMessages,
	parameters: {
		queries: [
			{ key: chatEntityKey(MockChat.id), data: mockSummaryChat },
			{ key: chatCostTreeKey(MockChat.id), data: mockCost },
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(await canvas.findByRole("button", { name: "Retry" }));
		await canvas.findByText("context deadline exceeded");
		onlineManager.setOnline(false);
		onlineManager.setOnline(true);
	},
};

const mockRunsFile: TypesGen.ChatFileMetadata = {
	...MockChatFileMetadata,
	id: "runs-file",
	name: "flaky-runs.csv",
	mime_type: "text/csv",
};

const mockRunsOnlyChat: TypesGen.Chat = {
	...MockChat,
	files: [mockRunsFile],
};

export const ExpiredAttachment: Story = {
	render: (args) => (
		<ChatSummaryPanel {...args} store={createAttachmentStore()} />
	),
	parameters: {
		queries: [
			{ key: chatEntityKey(MockChat.id), data: mockRunsOnlyChat },
			{ key: chatCostTreeKey(MockChat.id), data: mockCost },
		],
	},
};

const mockReportFile: TypesGen.ChatFileMetadata = {
	...MockChatFileMetadata,
	id: "report-file",
	name: "latency-report.csv",
	mime_type: "text/csv",
};

const mockReportOnlyChat: TypesGen.Chat = {
	...MockChat,
	files: [mockReportFile],
};

export const ExpiredEarlierAttachment: Story = {
	args: { hasMoreMessages: true },
	render: renderWithLatestMessages,
	parameters: {
		queries: [
			{ key: chatEntityKey(MockChat.id), data: mockReportOnlyChat },
			{ key: chatCostTreeKey(MockChat.id), data: mockCost },
			{
				key: chatAttachmentsKey(MockChat.id, mockLatestMessages[0].id),
				data: { attachments: [mockEarlierScreenshot, mockEarlierUpload] },
			},
		],
	},
};
