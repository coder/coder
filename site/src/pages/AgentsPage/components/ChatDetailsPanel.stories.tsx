import type { Meta, StoryObj } from "@storybook/react-vite";
import { onlineManager } from "react-query";
import { fn, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import {
	chatAttachmentsKey,
	chatCostTreeKey,
	chatEntityKey,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatContext, FeatureName } from "#/api/typesGenerated";
import {
	MockChat,
	MockChatAttachment,
	MockChatContextACPHarnesses,
	MockChatContextACPHarnessIssue,
	MockChatContextClean,
	MockChatContextDirty,
	MockChatFileMetadata,
	MockChatMessage,
} from "#/testHelpers/chatEntities";
import { mockApiError } from "#/testHelpers/entities";
import { MockPngBase64 } from "#/testHelpers/files";
import { withDashboardProvider } from "#/testHelpers/storybook";
import type { ResolvedCompactionThreshold } from "../utils/compactionTriggers";
import { createChatStore } from "./ChatConversation/chatStore";
import { attachmentStepMessages } from "./ChatConversation/storyFixtures";
import { ChatDetailsPanel } from "./ChatDetailsPanel";

const modelThreshold = (percent: number): ResolvedCompactionThreshold => ({
	percent,
	source: "model",
	organizationOverrideNotLoaded: false,
});

const mockSummaryChat: TypesGen.Chat = {
	...MockChat,
	summary:
		"Investigated the flaky CI job, traced it to a cache-layer race, and added a regression test.",
};

const mockCost: TypesGen.ChatCost = {
	chat_id: MockChat.id,
	total_cost_micros: 1_250_000,
	request_count: 8,
	unpriced_request_count: 0,
};

const PanelFrame = (Story: React.FC) => (
	<div className="h-[760px] w-[360px] max-w-full border border-solid border-border">
		<Story />
	</div>
);
const meta: Meta<typeof ChatDetailsPanel> = {
	title: "pages/AgentsPage/ChatDetailsPanel",
	component: ChatDetailsPanel,
	decorators: [PanelFrame, withDashboardProvider],
	parameters: {
		features: ["aibridge"] satisfies FeatureName[],
		queries: [
			{ key: chatEntityKey(MockChat.id), data: mockSummaryChat },
			{ key: chatCostTreeKey(MockChat.id), data: mockCost },
		],
	},
	args: {
		chatId: MockChat.id,
		isVisible: true,
		usage: {
			usedTokens: 26_000,
			contextLimitTokens: 1_100_000,
			compactionThreshold: modelThreshold(70),
			context: MockChatContextClean,
		},
		onApplyContext: fn(),
		hasMoreMessages: false,
		isHydratingMessages: false,
	},
	render: (args) => <ChatDetailsPanel {...args} store={createChatStore()} />,
};
export default meta;
type Story = StoryObj<typeof ChatDetailsPanel>;

const expandResources: Story["play"] = async ({ canvasElement }) => {
	const canvas = within(canvasElement);
	for (const name of [/^Context/, /^Skills/, /^MCP servers/])
		await userEvent.click(canvas.getByRole("button", { name }));
};

export const WithSummary: Story = {};
export const ExpandedResources: Story = { play: expandResources };
export const NoSummary: Story = {
	parameters: {
		queries: [
			{ key: chatEntityKey(MockChat.id), data: MockChat },
			{
				key: chatCostTreeKey(MockChat.id),
				data: {
					chat_id: MockChat.id,
					total_cost_micros: 0,
					request_count: 0,
					unpriced_request_count: 0,
				},
			},
		],
	},
};
export const SubagentSummaryPending: Story = {
	parameters: {
		queries: [
			{
				key: chatEntityKey(MockChat.id),
				data: { ...MockChat, parent_chat_id: "parent-chat-id" },
			},
			{
				key: chatCostTreeKey("parent-chat-id"),
				data: {
					chat_id: "parent-chat-id",
					total_cost_micros: 0,
					request_count: 0,
					unpriced_request_count: 0,
				},
			},
		],
	},
};
export const SubagentTreeCost: Story = {
	parameters: {
		queries: [
			{
				key: chatEntityKey(MockChat.id),
				data: {
					...MockChat,
					parent_chat_id: "parent-chat-id",
					root_chat_id: "root-chat-id",
				},
			},
			{
				key: chatCostTreeKey("root-chat-id"),
				data: {
					chat_id: "root-chat-id",
					total_cost_micros: 1_250_000,
					request_count: 8,
					unpriced_request_count: 1,
				},
			},
		],
	},
};
export const ChatError: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API.experimental, "getChat").mockRejectedValue(
			new Error("Failed to load chat"),
		);
	},
};
export const Loading: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API.experimental, "getChat").mockImplementation(
			() => new Promise(() => {}),
		);
	},
};
export const GatewayUnavailable: Story = { parameters: { features: [] } };
export const Dirty: Story = {
	args: { usage: { ...meta.args?.usage, context: MockChatContextDirty } },
};
export const Applying: Story = {
	args: { ...Dirty.args, isApplyingContext: true },
};
export const ApplyError: Story = {
	args: {
		...Dirty.args,
		applyError: new Error(
			"Workspace context is temporarily unavailable. Try again.",
		),
	},
};
export const Applied: Story = { args: { applySuccess: true } };
export const Archived: Story = {
	args: { ...Dirty.args, onApplyContext: undefined },
};
export const SnapshotError: Story = {
	args: {
		usage: {
			...meta.args?.usage,
			context: {
				...MockChatContextClean,
				error: "Failed to read AGENTS.md: permission denied",
			},
		},
	},
	play: expandResources,
};
export const NoUsage: Story = { args: { usage: null }, play: expandResources };
export const NoUsageCollapsed: Story = { args: NoUsage.args };
export const UsageWithoutInventory: Story = {
	args: { usage: { ...meta.args?.usage, context: undefined } },
};
export const UsageWithoutContextLimit: Story = {
	args: { usage: { usedTokens: 26_000, context: MockChatContextClean } },
	play: expandResources,
};
export const ZeroUsage: Story = {
	args: { usage: { ...meta.args?.usage, usedTokens: 0 } },
	play: expandResources,
};
export const EmptyContext: Story = {
	args: { usage: { context: { ...MockChatContextClean, resources: [] } } },
	play: expandResources,
};
export const UsageWithoutTokenCounts: Story = {
	args: {
		usage: { contextLimitTokens: 200_000, context: MockChatContextClean },
	},
	play: expandResources,
};
export const CompactionDisabled: Story = {
	args: {
		usage: { ...meta.args?.usage, compactionThreshold: modelThreshold(100) },
	},
	play: expandResources,
};
export const ZeroThreshold: Story = {
	args: {
		usage: { ...meta.args?.usage, compactionThreshold: modelThreshold(0) },
	},
	play: expandResources,
};
export const OrganizationOverride: Story = {
	args: {
		usage: {
			...meta.args?.usage,
			compactionThreshold: { source: "organization", pointTokens: 160_000 },
		},
	},
	play: expandResources,
};
export const EstimatedUsage: Story = {
	args: { usage: { ...meta.args?.usage, estimated: true } },
	play: expandResources,
};

const MockMultipleRoots: ChatContext = {
	...MockChatContextClean,
	resources: [
		...(MockChatContextClean.resources ?? []),
		{
			source: "/home/coder/project/site/AGENTS.md",
			kind: "instruction_file",
			size_bytes: 512,
			status: "ok",
		},
		{
			source: "/home/coder/project/.agents/skills/deploy",
			kind: "skill",
			size_bytes: 140,
			status: "ok",
			skill_name: "deploy",
			skill_description: "Deploy this project to staging.",
		},
	],
};
export const MultipleContextRoots: Story = {
	args: { usage: { ...meta.args?.usage, context: MockMultipleRoots } },
	play: expandResources,
};
const MockMcpContext: ChatContext = {
	...MockChatContextClean,
	resources: [
		...(MockChatContextClean.resources ?? []),
		{
			source: "/home/coder/project/.mcp.json",
			kind: "mcp_config",
			size_bytes: 256,
			status: "ok",
		},
		{
			source: "connected-without-tools",
			kind: "mcp_server",
			size_bytes: 0,
			status: "ok",
			tools: [],
		},
		{
			source: "unreachable",
			kind: "mcp_server",
			size_bytes: 0,
			status: "unreadable",
			error: "Connection refused: could not discover tools.",
		},
	],
};
export const MultipleMcpConfigs: Story = {
	args: { usage: { ...meta.args?.usage, context: MockMcpContext } },
	play: expandResources,
};
export const FailedMcpCollapsed: Story = { args: MultipleMcpConfigs.args };
const expandHarnesses: Story["play"] = async ({ canvasElement }) => {
	await userEvent.click(
		within(canvasElement).getByRole("button", { name: /^ACP harnesses/ }),
	);
};
// A harness without a source path renders outside any directory group.
const MockACPHarnessContext: ChatContext = {
	...MockChatContextClean,
	resources: [
		...(MockChatContextClean.resources ?? []),
		...(MockChatContextACPHarnesses.resources ?? []),
		{ source: "gemini", kind: "acp_harness", size_bytes: 0, status: "ok" },
	],
};
export const ACPHarnesses: Story = {
	args: { usage: { ...meta.args?.usage, context: MockACPHarnessContext } },
	play: expandHarnesses,
};
export const ACPHarnessIssue: Story = {
	args: {
		usage: { ...meta.args?.usage, context: MockChatContextACPHarnessIssue },
	},
	play: expandHarnesses,
};
export const WorkspaceConnected: Story = {
	args: { workspaceStatus: "connected" },
};
export const WorkspaceStopped: Story = {
	args: { ...MultipleMcpConfigs.args, workspaceStatus: "stopped" },
};
export const NarrowLongContent: Story = {
	decorators: [
		(Story) => (
			<div className="w-[280px] max-w-full">
				<Story />
			</div>
		),
	],
	args: {
		usage: {
			...meta.args?.usage,
			context: {
				...MockMultipleRoots,
				resources: [
					...(MockMultipleRoots.resources ?? []),
					{
						source:
							"/home/coder/project/very-long-directory-name-that-must-wrap-without-horizontal-scrolling/nested-directory/AGENTS.md",
						kind: "instruction_file",
						status: "ok",
						size_bytes: 16384,
					},
					{
						source:
							"server-with-a-very-long-name-that-must-not-overflow-the-panel",
						kind: "mcp_server",
						status: "ok",
						size_bytes: 128,
						tools: [
							{
								name: "tool_with_an_extremely_long_name_that_must_wrap",
								description:
									"A detailed tool description that remains readable without relying on a hover-only tooltip.",
							},
						],
					},
				],
			},
		},
	},
	play: expandResources,
};
export const EnlargedText: Story = {
	...NarrowLongContent,
	beforeEach: () => {
		const previous = document.documentElement.style.fontSize;
		document.documentElement.style.fontSize = "32px";
		return () => {
			document.documentElement.style.fontSize = previous;
		};
	},
};
export const LightResourceIssues: Story = {
	...NarrowLongContent,
	parameters: { themes: { themeOverride: "light" } },
};

export const KeyboardFocus: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		canvas.getByRole("button", { name: "Summary" }).focus();
		await userEvent.tab();
	},
};

const mockUploadPart: TypesGen.ChatFilePart = {
	type: "file",
	media_type: "text/plain",
	name: "uploaded-ci.log",
	file_id: "upload-file",
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
		file: {
			type: "file",
			media_type: "image/png",
			name: "ci-dashboard.png",
			file_id: "screenshot-file",
		},
		result: { data: "", mime_type: "image/png" },
	}),
];

const createAttachmentStore = (messages = mockAttachmentMessages) => {
	const store = createChatStore();
	store.replaceMessages(messages);
	return store;
};

export const WithAttachments: Story = {
	render: (args) => (
		<ChatDetailsPanel {...args} store={createAttachmentStore()} />
	),
};

export const LoadingMessages: Story = {
	args: { isHydratingMessages: true },
};

export const LiveMessageArriving: Story = {
	args: { isHydratingMessages: true },
	render: WithAttachments.render,
};

export const ExpiredAttachment: Story = {
	render: WithAttachments.render,
	parameters: {
		queries: [
			{
				key: chatEntityKey(MockChat.id),
				data: {
					...MockChat,
					files: [
						{
							...MockChatFileMetadata,
							id: "screenshot-file",
							name: "ci-dashboard.png",
							mime_type: "image/png",
						},
					],
				},
			},
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
	<ChatDetailsPanel
		{...args}
		store={createAttachmentStore(mockLatestMessages)}
	/>
);

const mockEarlierScreenshot: TypesGen.ChatAttachment = {
	...MockChatAttachment,
	message_id: 6,
	role: "assistant",
	file: {
		type: "file",
		media_type: "image/png",
		name: "ci-dashboard.png",
		data: MockPngBase64,
	},
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

export const EarlierAttachmentsWhileFinalPageHydrates: Story = {
	args: { isHydratingMessages: true },
	render: renderWithLatestMessages,
	parameters: WithEarlierAttachments.parameters,
};

export const EarlierAttachmentsLoading: Story = {
	args: { hasMoreMessages: true },
	beforeEach: () => {
		spyOn(API.experimental, "getChatAttachments").mockReturnValue(
			new Promise(() => {}),
		);
	},
	render: renderWithLatestMessages,
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
	// Keep Retry in view for the screenshot.
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(await canvas.findByRole("button", { name: "Retry" }));
		await canvas.findByText("context deadline exceeded");
		onlineManager.setOnline(false);
		onlineManager.setOnline(true);
	},
};

const mockReportOnlyChat: TypesGen.Chat = {
	...MockChat,
	files: [
		{
			...MockChatFileMetadata,
			id: "report-file",
			name: "latency-report.csv",
			mime_type: "text/csv",
		},
	],
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
