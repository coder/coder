import { MessageScroller } from "@shadcn/react/message-scroller";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import {
	expect,
	fireEvent,
	fn,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatFileMetadata } from "#/testHelpers/chatEntities";
import { getChatFileURL } from "../../utils/chatAttachments";
import { ChatMessageScroller } from "../ChatMessageScroller";
import { ConversationTimeline } from "./ConversationTimeline";
import { parseMessagesWithMergedTools } from "./messageParsing";
import type { ParsedMessageEntry } from "./types";

// The timeline renders scroller items, so every story needs the scroller
// around it. Stories that exercise scrolling set `messageScrollerHeight` to
// bound the viewport; the rest render at their natural height.
const withMessageScroller: Decorator = (Story, { parameters }) => {
	const height =
		typeof parameters.messageScrollerHeight === "number"
			? parameters.messageScrollerHeight
			: undefined;
	return (
		<div className="flex flex-col" style={{ height }}>
			<MessageScroller.Provider autoScroll defaultScrollPosition="end">
				<ChatMessageScroller
					hasMoreMessages={false}
					isFetchingMoreMessages={false}
					isHydratingMessages={false}
					hasFetchMoreError={false}
					hasTranscriptRows={true}
					onFetchMoreMessages={async () => {}}
				>
					<Story />
				</ChatMessageScroller>
			</MessageScroller.Provider>
		</div>
	);
};

// 1×1 solid coral (#FF6B6B) PNG encoded as base64.
const TEST_PNG_B64 =
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg==";

const buildMessages = (messages: TypesGen.ChatMessage[]) =>
	parseMessagesWithMergedTools(messages);

const baseMessage = {
	chat_id: "story-chat",
	created_at: "2026-03-10T00:00:00.000Z",
} as const;

const askUserQuestionPayload = {
	questions: [
		{
			header: "Implementation Approach",
			question: "How should we structure the database migration?",
			options: [
				{
					label: "Single migration",
					description:
						"One migration file with all changes. Simpler but harder to roll back.",
				},
				{
					label: "Incremental migrations",
					description:
						"Split into multiple sequential migrations. More flexible rollback.",
				},
			],
		},
		{
			header: "Release Plan",
			question: "Which rollout path should we use for the new agent workflow?",
			options: [
				{
					label: "Internal dry run",
					description:
						"Ship to the team first and confirm the migration flow before broader rollout.",
				},
				{
					label: "Small beta",
					description:
						"Start with a limited set of workspaces so we can gather feedback quickly.",
				},
			],
		},
	],
};

const askUserQuestionSubmittedResponse = [
	"1. Implementation Approach: Incremental migrations",
	"2. Release Plan: Small beta",
].join("\n");

type AttachmentResponse = {
	status: number;
	body: string;
	contentType?: string;
};

const FAILED_ATTACHMENT_API_MESSAGE = "Failed to get chat file.";

const UNDISPLAYABLE_REMOTE_ATTACHMENT_MESSAGE =
	"File exists but could not be displayed.";

const ATTACHMENT_RESPONSES = new Map<string, AttachmentResponse>([
	[
		"storybook-test-text",
		{
			status: 200,
			body: "Quarterly revenue increased 18% year over year after the new pricing rollout stabilized customer expansion.",
		},
	],
	[
		"storybook-json-text",
		{ status: 200, body: '{"status":"ok","items":[1,2,3]}' },
	],
	[
		"storybook-text-only",
		{
			status: 200,
			body: "Runbook note: restart the worker after updating the queue configuration to pick up the new concurrency limits.",
		},
	],
	[
		"storybook-text-1",
		{
			status: 200,
			body: "First context file: deployment checklist and rollback instructions for the release candidate.",
		},
	],
	[
		"storybook-text-2",
		{
			status: 200,
			body: "Second context file: service logs showing a transient timeout while the cache warmed up.",
		},
	],
	[
		"storybook-text-3",
		{
			status: 200,
			body: "Third context file: local development configuration overrides for reproducing the issue.",
		},
	],
	["storybook-expired-image", { status: 404, body: "" }],
	["storybook-undisplayable-image", { status: 200, body: "" }],
	[
		"storybook-failed-image",
		{
			status: 500,
			body: JSON.stringify({
				message: FAILED_ATTACHMENT_API_MESSAGE,
				detail: "db: connection reset",
			}),
			contentType: "application/json",
		},
	],
	["storybook-expired-text", { status: 404, body: "" }],
	["storybook-expired-file", { status: 404, body: "" }],
	[
		"storybook-failed-text",
		{
			status: 500,
			body: JSON.stringify({
				message: FAILED_ATTACHMENT_API_MESSAGE,
				detail: "db: connection reset",
			}),
			contentType: "application/json",
		},
	],
	["storybook-text-error", { body: "Temporary failure", status: 503 }],
	[
		"storybook-ios-share-report",
		{ status: 200, body: "pdf-bytes", contentType: "application/pdf" },
	],
	["storybook-ios-error-report", { status: 500, body: "" }],
]);

let attachmentFetchCounts = new Map<string, number>();

const recordAttachmentFetch = (fileId: string) => {
	attachmentFetchCounts.set(
		fileId,
		(attachmentFetchCounts.get(fileId) ?? 0) + 1,
	);
};

const getAttachmentFetchCount = (fileId: string) =>
	attachmentFetchCounts.get(fileId) ?? 0;

const mockAttachmentFetch = () => {
	const originalFetch = globalThis.fetch;
	spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
		const url =
			typeof input === "string"
				? input
				: input instanceof URL
					? input.toString()
					: input.url;

		for (const [fileId, response] of ATTACHMENT_RESPONSES) {
			if (url.endsWith(fileId)) {
				recordAttachmentFetch(fileId);
				return new Response(response.body, {
					status: response.status,
					headers: response.contentType
						? { "Content-Type": response.contentType }
						: undefined,
				});
			}
		}

		return originalFetch(input, init);
	});
};

const buildTextPart = (text: string): TypesGen.ChatTextPart => ({
	type: "text",
	text,
});

const buildFilePart = (
	part: Omit<TypesGen.ChatFilePart, "type">,
): TypesGen.ChatFilePart => ({
	type: "file",
	...part,
});

const buildTextAttachmentPart = (fileId: string): TypesGen.ChatFilePart =>
	buildFilePart({ file_id: fileId, media_type: "text/plain" });

const buildImageAttachmentPart = (
	fileId: string,
	mediaType = "image/png",
): TypesGen.ChatFilePart =>
	buildFilePart({ file_id: fileId, media_type: mediaType });

const buildInlineAttachmentPart = (
	mediaType: string,
	data: string,
): TypesGen.ChatFilePart => buildFilePart({ media_type: mediaType, data });

const buildUserMessage = ({
	id = 1,
	text,
	files = [],
	createdAt = baseMessage.created_at,
}: {
	id?: number;
	text?: string;
	files?: TypesGen.ChatFilePart[];
	createdAt?: string;
}): TypesGen.ChatMessage => ({
	...baseMessage,
	created_at: createdAt,
	id,
	role: "user",
	content: [...(text ? [buildTextPart(text)] : []), ...files],
});

const buildStoryArgs = (...messages: TypesGen.ChatMessage[]) => ({
	...defaultArgs,
	parsedMessages: buildMessages(messages),
});

const buildChatFiles = (...fileIds: string[]): TypesGen.ChatFileMetadata[] =>
	fileIds.map((id) => ({ ...MockChatFileMetadata, id }));

const buildParsedReadFileEntry = ({
	messageId,
	toolId,
	path,
	status,
	content = "",
	errorMessage,
	isError = status === "error",
	hookRewritten = false,
}: {
	messageId: number;
	toolId: string;
	path: string;
	status: "completed" | "error" | "running";
	content?: string;
	errorMessage?: string;
	isError?: boolean;
	hookRewritten?: boolean;
}): ParsedMessageEntry => {
	const args = { path };
	const result =
		content || errorMessage
			? {
					...(content ? { content } : {}),
					...(errorMessage ? { error: errorMessage } : {}),
				}
			: undefined;

	return {
		message: {
			...baseMessage,
			id: messageId,
			role: "assistant",
			content: [
				{
					type: "tool-call",
					tool_call_id: toolId,
					tool_name: "read_file",
					args,
				},
			],
		},
		parsed: {
			markdown: "",
			reasoning: "",
			toolCalls: [{ id: toolId, name: "read_file", args }],
			toolResults: [],
			tools: [
				{
					id: toolId,
					name: "read_file",
					args,
					result,
					isError,
					status,
					hookRewritten,
				},
			],
			blocks: [{ type: "tool", id: toolId }],
			sources: [],
			hookNotices: [],
		},
	};
};

const buildReadFileExchange = (
	callMessageId: number,
	toolId: string,
	path: string,
	content: string,
): TypesGen.ChatMessage[] => {
	const args = { path };
	return [
		{
			...baseMessage,
			id: callMessageId,
			role: "assistant",
			content: [
				{
					type: "tool-call",
					tool_call_id: toolId,
					tool_name: "read_file",
					args,
				},
			],
		},
		{
			...baseMessage,
			id: callMessageId + 1,
			role: "tool",
			content: [
				{
					type: "tool-result",
					tool_call_id: toolId,
					tool_name: "read_file",
					result: { content },
				},
			],
		},
	];
};

const LONG_USER_MESSAGE = [
	"This is a deliberately long user message that should stay pinned to the",
	"right edge while the bubble stops short of filling the entire timeline",
	"column. It gives the Storybook test enough content to exercise the",
	"maximum width cap.",
].join(" ");

const findAttachmentTile = async (
	canvas: ReturnType<typeof within>,
	label: string,
) => {
	const tile = await canvas.findByRole("img", { name: label });
	expect(canvas.getByText(label)).toBeInTheDocument();
	return tile;
};

const expectNoCopyMessageButtonForElement = (element: HTMLElement) => {
	const messageRow = element.closest(
		'[data-role="user"], [data-role="assistant"]',
	);
	expect(messageRow).not.toBeNull();
	const messageWrapper = messageRow?.parentElement;
	expect(messageWrapper).not.toBeNull();
	if (!messageWrapper) {
		return;
	}
	expect(
		within(messageWrapper).queryByRole("button", {
			name: "Copy message",
		}),
	).not.toBeInTheDocument();
};

const hoverAndExpectTooltip = async (
	element: HTMLElement,
	text: RegExp | string,
) => {
	await userEvent.hover(element);
	const tooltip = await screen.findByRole("tooltip");
	expect(tooltip).toHaveTextContent(text);
	return tooltip;
};

const waitForTooltipWrappedAttachmentTile = async (
	canvas: ReturnType<typeof within>,
	label: string,
) => {
	await waitFor(() =>
		expect(canvas.getByRole("img", { name: label })).toHaveAttribute(
			"data-state",
		),
	);
	return canvas.getByRole("img", { name: label });
};

const defaultArgs: Omit<
	React.ComponentProps<typeof ConversationTimeline>,
	"parsedMessages"
> = {
	organizationId: "organization-id",
	subagentTitles: new Map(),
};

const meta: Meta<typeof ConversationTimeline> = {
	title: "pages/AgentsPage/ChatConversation/ConversationTimeline",
	component: ConversationTimeline,
	decorators: [withMessageScroller],
	beforeEach: () => {
		attachmentFetchCounts = new Map();
		mockAttachmentFetch();
	},
};
export default meta;
type Story = StoryObj<typeof ConversationTimeline>;

export const LifecycleHookNotice: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "system",
				content: [
					{
						type: "hook-notice",
						text: "Your organization requires an approval before deployment.",
					},
				],
			},
		]),
	},
};

export const SystemMessageWithoutHookNotice: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "system",
				content: [{ type: "text", text: "Maintenance starts in ten minutes." }],
			},
		]),
	},
};

export const LifecycleHookNoticeOnUserMessage: Story = {
	args: {
		...defaultArgs,
		urlTransform: (url) =>
			url.replace("http://localhost:3000", "https://proxy.example.com"),
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "original prompt" },
					{
						type: "hook-notice",
						text: "Deployment context was added: [policy](http://localhost:3000/policy)",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const notice = canvas.getByRole("note");
		expect(notice).toBeVisible();
		expect(within(notice).getByText("Lifecycle hook")).toBeVisible();
		const prompt = canvas.getByText("original prompt");
		expect(prompt).toBeVisible();
		expect(
			prompt.compareDocumentPosition(notice) & Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
		const link = within(notice).getByRole("link", { name: "policy" });
		expect(link).toHaveAttribute("href", "https://proxy.example.com/policy");
	},
};

export const LifecycleHookNoticeAfterEditedMessage: Story = {
	args: {
		...defaultArgs,
		editingMessageId: 1,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "prompt being edited" }],
			},
			{
				...baseMessage,
				id: 2,
				role: "user",
				content: [
					{ type: "text", text: "later prompt" },
					{
						type: "hook-notice",
						text: "Deployment context was added: [policy](http://localhost:3000/policy)",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("prompt being edited")).toBeVisible();
		const link = canvas.getByRole("link", { name: "policy" });
		link.focus();
		expect(link).not.toHaveFocus();
	},
};

export const FindToolsSearchResult: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "assistant",
				content: [
					{
						type: "tool-call",
						tool_call_id: "find-tools-1",
						tool_name: "find_tools",
						args: {
							queries: JSON.stringify(["github issues", "pull requests"]),
							names: JSON.stringify(["github__list_issues"]),
						},
					},
				],
			},
			{
				...baseMessage,
				id: 2,
				role: "tool",
				content: [
					{
						type: "tool-result",
						tool_call_id: "find-tools-1",
						tool_name: "find_tools",
						result: {
							matches: JSON.stringify([
								{
									name: "github__list_issues",
									description: "List issues in a GitHub repository.",
								},
								{
									name: "github__list_pull_requests",
									description: "List pull requests in a GitHub repository.",
								},
							]),
							activated: JSON.stringify([
								"github__list_issues",
								"github__list_pull_requests",
							]),
							total_deferred: "24",
						},
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const summary = canvas.getByRole("button", {
			name: "Matched 2 tools: github issues, pull requests, github__list_issues",
		});
		expect(summary).toBeVisible();
		expect(canvas.queryByText("github__list_issues")).not.toBeInTheDocument();
		await userEvent.click(summary);
		expect(canvas.getByText("github__list_issues")).toBeVisible();
		expect(
			canvas.getByText("List issues in a GitHub repository."),
		).toBeVisible();
		expect(canvas.getByText("github__list_pull_requests")).toBeVisible();
		expect(
			canvas.getByText("List pull requests in a GitHub repository."),
		).toBeVisible();
	},
};

export const FindToolsEmptyResult: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "assistant",
				content: [
					{
						type: "tool-call",
						tool_call_id: "find-tools-empty",
						tool_name: "find_tools",
						args: { queries: JSON.stringify(["nonexistent capability"]) },
					},
				],
			},
			{
				...baseMessage,
				id: 2,
				role: "tool",
				content: [
					{
						type: "tool-result",
						tool_call_id: "find-tools-empty",
						tool_name: "find_tools",
						result: {
							matches: JSON.stringify([]),
							activated: JSON.stringify([]),
							total_deferred: "24",
						},
					},
				],
			},
		]),
	},
};

export const FindToolsErrorResult: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "assistant",
				content: [
					{
						type: "tool-call",
						tool_call_id: "find-tools-error",
						tool_name: "find_tools",
						args: { queries: JSON.stringify(["github issues"]) },
					},
				],
			},
			{
				...baseMessage,
				id: 2,
				role: "tool",
				content: [
					{
						type: "tool-result",
						tool_call_id: "find-tools-error",
						tool_name: "find_tools",
						is_error: true,
						result: {
							error:
								"The schema budget for this step is exhausted; call the tools already activated or retry next step.",
						},
					},
				],
			},
		]),
	},
};

export const FindToolsMalformedResultUsesDefaultRenderer: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "assistant",
				content: [
					{
						type: "tool-call",
						tool_call_id: "find-tools-invalid",
						tool_name: "find_tools",
						args: { queries: JSON.stringify(["github"]) },
					},
				],
			},
			{
				...baseMessage,
				id: 2,
				role: "tool",
				content: [
					{
						type: "tool-result",
						tool_call_id: "find-tools-invalid",
						tool_name: "find_tools",
						result: { matches: "not-json" },
					},
				],
			},
		]),
	},
};

export const DurableListTemplatesToolLifecycle: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "Show me available templates" }],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [
					{
						type: "tool-call",
						tool_call_id: "list-templates-1",
						tool_name: "list_templates",
						args: {},
					},
				],
			},
			{
				...baseMessage,
				id: 3,
				role: "tool",
				content: [
					{
						type: "tool-result",
						tool_call_id: "list-templates-1",
						tool_name: "list_templates",
						result: {
							count: "1",
							templates:
								'[{"id":"template-1","name":"docker","display_name":"Docker"}]',
						},
					},
				],
			},
		]),
	},
};

/**
 * User bubbles should stay right-aligned, shrink to fit short content,
 * and cap long content so the timeline keeps some breathing room.
 */
export const UserMessageBubbleAlignment: Story = {
	args: buildStoryArgs(
		buildUserMessage({
			text: LONG_USER_MESSAGE,
		}),
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const messageText = canvas.getByText(/deliberately long user message/i);
		const userRow = messageText.closest('[data-role="user"]');
		expect(userRow).not.toBeNull();

		const bubble = userRow?.firstElementChild;
		expect(bubble).not.toBeNull();

		await userEvent.hover(userRow?.parentElement as HTMLElement);
		const actions = await canvas.findByTestId("message-actions");

		const rowRect = (userRow as HTMLElement).getBoundingClientRect();
		const bubbleRect = (bubble as HTMLElement).getBoundingClientRect();
		const actionsRect = actions.getBoundingClientRect();

		expect(bubbleRect.width).toBeLessThanOrEqual(rowRect.width * 0.81);
		expect(Math.abs(rowRect.right - bubbleRect.right)).toBeLessThanOrEqual(2);
		expect(Math.abs(rowRect.right - actionsRect.right)).toBeLessThanOrEqual(2);
	},
};

/** Regression guard: a single image attachment must not be duplicated. */
export const UserMessageWithSingleImage: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Check this screenshot" },
					{
						type: "file",
						media_type: "image/png",
						data: TEST_PNG_B64,
					},
				],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [
					{
						type: "text",
						text: "I can see the screenshot. It looks like a settings panel.",
					},
				],
			},
		]),
	},
};

/** Ensures N images in yields exactly N thumbnails with no duplication. */
export const UserMessageWithMultipleImages: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Here are three screenshots" },
					{
						type: "file",
						media_type: "image/png",
						data: TEST_PNG_B64,
					},
					{
						type: "file",
						media_type: "image/jpeg",
						data: TEST_PNG_B64,
					},
					{
						type: "file",
						media_type: "image/webp",
						data: TEST_PNG_B64,
					},
				],
			},
		]),
	},
};

/** File-id images use a server URL instead of inline base64 data. */
export const UserMessageWithFileIdImage: Story = {
	args: buildStoryArgs(
		buildUserMessage({
			text: "Uploaded via file ID",
			files: [buildImageAttachmentPart("storybook-test-image")],
		}),
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.getAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(1);
		// Verify file_id path is used, not a base64 data URI.
		expect(images[0]).toHaveAttribute(
			"src",
			getChatFileURL("storybook-test-image"),
		);
		expectNoCopyMessageButtonForElement(images[0]);
	},
};

/** File-id images that probe as 404 render an expired placeholder. */
export const UserMessageWithExpiredImage: Story = {
	args: buildStoryArgs(
		buildUserMessage({
			text: "This upload has expired",
			files: [buildImageAttachmentPart("storybook-expired-image")],
		}),
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const image = canvas.getByRole("img", { name: "Attached image" });
		fireEvent.error(image);
		const expiredTile = await findAttachmentTile(canvas, "Image expired");
		expect(canvas.getByText("This upload has expired")).toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "View Attached image" }),
		).not.toBeInTheDocument();
		expectNoCopyMessageButtonForElement(expiredTile);

		// The tooltip names the attachment cap and describes retention
		// generically so the copy survives any operator-chosen window.
		await hoverAndExpectTooltip(
			expiredTile,
			/keeps its 50 most recent attachments/i,
		);
	},
};

/** Duplicate expired file IDs reuse the first probe result page-wide. */
export const UserMessageWithRepeatedExpiredImage: Story = {
	args: buildStoryArgs(
		buildUserMessage({
			id: 1,
			text: "First reference to the expired upload",
			files: [buildImageAttachmentPart("storybook-expired-image")],
		}),
		buildUserMessage({
			id: 2,
			text: "Second reference to the same expired upload",
			files: [buildImageAttachmentPart("storybook-expired-image")],
		}),
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.getAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(2);
		fireEvent.error(images[0]);
		fireEvent.error(images[1]);
		await waitFor(() =>
			expect(
				canvas.getAllByRole("img", { name: "Image expired" }),
			).toHaveLength(2),
		);
		expect(getAttachmentFetchCount("storybook-expired-image")).toBe(1);
		expect(
			canvas.queryByRole("button", { name: "View Attached image" }),
		).not.toBeInTheDocument();
		for (const tile of canvas.getAllByRole("img", { name: "Image expired" })) {
			expectNoCopyMessageButtonForElement(tile);
		}
	},
};

/** Duplicate file IDs with a non-expired probe reuse the first result. */
export const UserMessageWithRepeatedFailedImage: Story = {
	args: buildStoryArgs(
		buildUserMessage({
			id: 1,
			text: "First reference to the failed upload",
			files: [buildImageAttachmentPart("storybook-failed-image")],
		}),
		buildUserMessage({
			id: 2,
			text: "Second reference to the same failed upload",
			files: [buildImageAttachmentPart("storybook-failed-image")],
		}),
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.getAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(2);
		fireEvent.error(images[0]);
		fireEvent.error(images[1]);
		await waitFor(() =>
			expect(
				canvas.getAllByRole("img", { name: "Image failed to load" }),
			).toHaveLength(2),
		);
		expect(getAttachmentFetchCount("storybook-failed-image")).toBe(1);
		expect(
			canvas.queryByRole("button", { name: "View Attached image" }),
		).not.toBeInTheDocument();

		const tiles = await waitFor(() => {
			const t = canvas.getAllByRole("img", { name: "Image failed to load" });
			for (const tile of t) {
				expect(tile).toHaveAttribute("data-state");
			}
			return t;
		});
		for (const tile of tiles) {
			expectNoCopyMessageButtonForElement(tile);
			await hoverAndExpectTooltip(tile, FAILED_ATTACHMENT_API_MESSAGE);
		}
	},
};

/** File-id images that fail with a non-404 status render a generic failure tile. */
export const UserMessageWithFailedRemoteImage: Story = {
	args: buildStoryArgs(
		buildUserMessage({
			text: "This image failed to load",
			files: [buildImageAttachmentPart("storybook-failed-image")],
		}),
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const image = canvas.getByRole("img", { name: "Attached image" });
		fireEvent.error(image);
		const failedTile = await findAttachmentTile(canvas, "Image failed to load");
		expect(canvas.getByText("This image failed to load")).toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "View Attached image" }),
		).not.toBeInTheDocument();
		expectNoCopyMessageButtonForElement(failedTile);

		// When the probe returns a structured error body, the tooltip
		// surfaces the API's message so the viewer has something
		// actionable instead of a bare "failed to load". The label
		// doesn't change when the probe settles (still "Image failed
		// to load"), and the tile's DOM node is replaced when the
		// Tooltip wrapper mounts, so re-query each time and wait for
		// the Radix-stamped data-state attribute before hovering.
		await hoverAndExpectTooltip(
			await waitForTooltipWrappedAttachmentTile(canvas, "Image failed to load"),
			FAILED_ATTACHMENT_API_MESSAGE,
		);
	},
};

/** A successful follow-up probe still maps to the generic failure tile. */
export const UserMessageWithUndisplayableRemoteImage: Story = {
	args: buildStoryArgs(
		buildUserMessage({
			text: "This image exists but cannot be displayed",
			files: [buildImageAttachmentPart("storybook-undisplayable-image")],
		}),
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const image = canvas.getByRole("img", { name: "Attached image" });
		fireEvent.error(image);
		const failedTile = await findAttachmentTile(canvas, "Image failed to load");
		expectNoCopyMessageButtonForElement(failedTile);
		await hoverAndExpectTooltip(
			await waitForTooltipWrappedAttachmentTile(canvas, "Image failed to load"),
			UNDISPLAYABLE_REMOTE_ATTACHMENT_MESSAGE,
		);
	},
};

/** Invalid inline image data skips the probe and renders the generic failure tile. */
export const UserMessageWithInvalidInlineImage: Story = {
	args: buildStoryArgs(
		buildUserMessage({
			text: "Inline image data is corrupt",
			files: [buildInlineAttachmentPart("image/png", "not-valid-base64")],
		}),
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const image = canvas.getByRole("img", { name: "Attached image" });
		fireEvent.error(image);
		const failedTile = await findAttachmentTile(canvas, "Image failed to load");
		expect(
			canvas.getByText("Inline image data is corrupt"),
		).toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "View Attached image" }),
		).not.toBeInTheDocument();
		expectNoCopyMessageButtonForElement(failedTile);
	},
};

export const UserMessageWithTextAttachment: Story = {
	args: buildStoryArgs(
		buildUserMessage({
			text: "Here is some context from our docs:",
			files: [buildTextAttachmentPart("storybook-test-text")],
		}),
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const textButton = await canvas.findByRole("button", {
			name: "View text attachment",
		});
		expect(textButton).toBeInTheDocument();
		expect(textButton).toHaveTextContent(/Pasted text/i);
		expect(
			canvas.queryByRole("button", { name: "Copy message" }),
		).not.toBeInTheDocument();
		await userEvent.click(textButton);
		expect(
			await canvas.findByText(/Quarterly revenue increased 18%/i),
		).toBeInTheDocument();
	},
};

export const UserMessageWithJSONAttachment: Story = {
	args: {
		...defaultArgs,
		parsedMessages: parseMessagesWithMergedTools([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Here is the structured report." },
					{
						type: "file",
						file_id: "storybook-json-text",
						media_type: "application/json",
						name: "report.json",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const textButton = await canvas.findByRole("button", {
			name: "View report.json",
		});
		expect(textButton).toHaveTextContent("report.json");
		expect(
			canvas.queryByRole("button", { name: "Copy message" }),
		).not.toBeInTheDocument();
		await userEvent.click(textButton);
		expect(await canvas.findByText(/"status":"ok"/i)).toBeInTheDocument();
	},
};

export const UserMessageWithDownloadableFile: Story = {
	args: {
		...defaultArgs,
		parsedMessages: parseMessagesWithMergedTools([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "I attached the deployment report." },
					{
						type: "file",
						media_type: "application/pdf",
						file_id: "storybook-user-deployment-report",
						name: "deployment-report.pdf",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const downloadLink = canvas.getByRole("link", {
			name: "Download deployment-report.pdf",
		});
		expect(downloadLink).toHaveAttribute(
			"href",
			"/api/v2/chats/files/storybook-user-deployment-report",
		);
		expect(canvas.getByText("deployment-report.pdf")).toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Copy message" }),
		).not.toBeInTheDocument();
	},
};

export const UserMessageWithMultipleTextAttachments: Story = {
	args: buildStoryArgs(
		buildUserMessage({
			createdAt: "2025-01-15T10:00:00Z",
			text: "Here are several context files:",
			files: [
				buildTextAttachmentPart("storybook-text-1"),
				buildTextAttachmentPart("storybook-text-2"),
				buildTextAttachmentPart("storybook-text-3"),
			],
		}),
	),
};

export const UserMessageWithTextAttachmentOnly: Story = {
	args: buildStoryArgs(
		buildUserMessage({
			files: [buildTextAttachmentPart("storybook-text-only")],
		}),
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const textButton = await canvas.findByRole("button", {
			name: "View text attachment",
		});
		expect(textButton).toHaveTextContent(/Pasted text/i);
		expectNoCopyMessageButtonForElement(textButton);
		await userEvent.click(textButton);
		expect(
			await canvas.findByText(/Runbook note: restart the worker/i),
		).toBeInTheDocument();
	},
};

/**
 * A text attachment the chat record no longer lists, referenced before a
 * remaining one, renders the placeholder on load without fetching the file.
 */
export const UserMessageWithExpiredTextAttachment: Story = {
	args: {
		...buildStoryArgs(
			buildUserMessage({
				id: 1,
				text: "This pasted context has expired",
				files: [buildTextAttachmentPart("storybook-expired-text")],
			}),
			buildUserMessage({
				id: 2,
				text: "This newer context is still available",
				files: [buildTextAttachmentPart("storybook-test-text")],
			}),
		),
		chatFiles: buildChatFiles("storybook-test-text"),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const expiredTile = await findAttachmentTile(canvas, "Attachment expired");
		expect(
			canvas.getByText("This pasted context has expired"),
		).toBeInTheDocument();
		expect(
			canvas.getAllByRole("button", { name: "View text attachment" }),
		).toHaveLength(1);
		expectNoCopyMessageButtonForElement(expiredTile);

		await hoverAndExpectTooltip(
			expiredTile,
			/keeps its 50 most recent attachments/i,
		);
	},
};

/** An evicted downloadable file renders the placeholder instead of a dead link. */
export const UserMessageWithExpiredDownloadableFile: Story = {
	args: {
		...buildStoryArgs(
			buildUserMessage({
				id: 1,
				text: "The attached report has expired.",
				files: [
					buildFilePart({
						media_type: "application/pdf",
						file_id: "storybook-expired-file",
						name: "old-report.pdf",
					}),
				],
			}),
			buildUserMessage({
				id: 2,
				text: "The newer report is still available.",
				files: [
					buildFilePart({
						media_type: "application/pdf",
						file_id: "storybook-current-file",
						name: "new-report.pdf",
					}),
				],
			}),
		),
		chatFiles: buildChatFiles("storybook-current-file"),
	},
};

export const UserMessageWithFailedTextAttachment: Story = {
	args: buildStoryArgs(
		buildUserMessage({
			text: "This pasted context failed to load",
			files: [buildTextAttachmentPart("storybook-failed-text")],
		}),
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const textButton = await canvas.findByRole("button", {
			name: "View text attachment",
		});
		expectNoCopyMessageButtonForElement(textButton);
		await userEvent.click(textButton);
		await findAttachmentTile(canvas, "Attachment failed to load");
		expect(
			canvas.getByText("This pasted context failed to load"),
		).toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "View text attachment" }),
		).not.toBeInTheDocument();

		await hoverAndExpectTooltip(
			await waitForTooltipWrappedAttachmentTile(
				canvas,
				"Attachment failed to load",
			),
			FAILED_ATTACHMENT_API_MESSAGE,
		);
	},
};

/**
 * Non-JSON error bodies (a bare `Temporary failure` text body with status 503)
 * still surface the shared failure tile, and the raw body must not leak into
 * the message stream where it would look like assistant content.
 */
export const UserMessageWithFailedTextAttachmentNonJSONBody: Story = {
	args: {
		...defaultArgs,
		parsedMessages: parseMessagesWithMergedTools([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "The preview fetch will fail." },
					{
						type: "file",
						file_id: "storybook-text-error",
						media_type: "text/plain",
						name: "preview.txt",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const textButton = await canvas.findByRole("button", {
			name: "View preview.txt",
		});
		expectNoCopyMessageButtonForElement(textButton);
		await userEvent.click(textButton);
		await findAttachmentTile(canvas, "Attachment failed to load");
		expect(
			canvas.queryByRole("button", { name: "View preview.txt" }),
		).not.toBeInTheDocument();
		expect(canvas.queryByText(/Temporary failure/i)).not.toBeInTheDocument();
	},
};

/** Visual regression: text and image attachments render at the same height. */
export const UserMessageWithMixedAttachments: Story = {
	args: buildStoryArgs(
		buildUserMessage({
			text: "Here is a screenshot and some context",
			files: [
				buildInlineAttachmentPart("image/png", TEST_PNG_B64),
				buildTextAttachmentPart("storybook-test-text"),
			],
		}),
	),
};

/** Text-only messages must not produce spurious image thumbnails. */
export const UserMessageTextOnly: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "Just a plain text message" }],
			},
		]),
	},
};

/** Assistant-side images go through BlockList, not the user path. */
export const AssistantMessageWithImage: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "assistant",
				content: [
					{ type: "text", text: "Here is the generated image:" },
					{
						type: "file",
						media_type: "image/png",
						data: TEST_PNG_B64,
						name: "generated-image.png",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.getAllByRole("img", { name: "generated-image.png" });
		expect(images).toHaveLength(1);
		expect(images[0]).toHaveAttribute(
			"src",
			`data:image/png;base64,${TEST_PNG_B64}`,
		);
		expect(
			canvas.queryByRole("link", { name: "Download generated-image.png" }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Copy message" }),
		).not.toBeInTheDocument();
		const viewButton = canvas.getByRole("button", {
			name: "View generated-image.png",
		});
		viewButton.focus();
		expect(viewButton).toHaveFocus();
		await waitFor(() => {
			expect(
				canvas.getByRole("link", { name: "Download generated-image.png" }),
			).toBeVisible();
		});
	},
};

export const AssistantMessageWithUnnamedDownloadableFile: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "assistant",
				content: [
					{ type: "text", text: "I attached the file without a custom name." },
					{
						type: "file",
						media_type: "application/pdf",
						file_id: "storybook-unnamed-report",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const downloadLink = canvas.getByRole("link", {
			name: "Download Attached file",
		});
		expect(downloadLink).toBeInTheDocument();
		expect(downloadLink).toHaveAttribute("download", "attachment.pdf");
		expect(canvas.getByText("Attached file")).toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Copy message" }),
		).not.toBeInTheDocument();
	},
};

export const AssistantMessageWithMismatchedExtensionFile: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "assistant",
				content: [
					{ type: "text", text: "Here are the release notes." },
					{
						type: "file",
						media_type: "application/pdf",
						file_id: "storybook-mismatched-notes",
						name: "release-notes.txt",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const downloadLink = canvas.getByRole("link", {
			name: "Download release-notes.txt",
		});
		expect(downloadLink).toHaveAttribute("download", "release-notes.txt");
	},
};

/** Images and file-references coexist without interfering. */
export const UserMessageWithImagesAndFileRefs: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Look at these files" },
					{
						type: "file",
						media_type: "image/png",
						data: TEST_PNG_B64,
					},
					{
						type: "file-reference",
						file_name: "src/main.go",
						start_line: 10,
						end_line: 25,
						content: 'func main() {\n\tfmt.Println("hello")\n}',
					},
				],
			},
		]),
	},
};

/** File references render inline with text, matching the chat input style. */
export const UserMessageWithInlineFileRef: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Can you refactor " },
					{
						type: "file-reference",
						file_name: "site/src/components/Button.tsx",
						start_line: 42,
						end_line: 42,
						content: "export const Button = ...",
					},
					{ type: "text", text: " to use the new API?" },
				],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [
					{
						type: "text",
						text: "Sure, I'll update that component.",
					},
				],
			},
		]),
	},
};

export const MetadataOnlyUserMessageRendersNoRow: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "assistant",
				content: [{ type: "text", text: "Before hidden metadata." }],
			},
			{
				...baseMessage,
				id: 2,
				role: "user",
				content: [
					{
						type: "context-file",
						context_file_path: "/home/coder/coder/AGENTS.md",
					},
				],
			},
			{
				...baseMessage,
				id: 3,
				role: "assistant",
				content: [{ type: "text", text: "After hidden metadata." }],
			},
		]),
	},
};

/**
 * Each user prompt is a single transcript row. The scroller anchors on those
 * rows, so nothing renders a second, pinned copy of the prompt.
 */
export const UserMessagesRenderAsSingleRows: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "First prompt" }],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [{ type: "text", text: "First response" }],
			},
			{
				...baseMessage,
				id: 3,
				role: "user",
				content: [{ type: "text", text: "Second prompt" }],
			},
			{
				...baseMessage,
				id: 4,
				role: "assistant",
				content: [{ type: "text", text: "Second response" }],
			},
		]),
	},
};

/**
 * Each user message exposes left/right chevron buttons in its action row so
 * users can jump the transcript between user prompts. They are disabled at the
 * ends of the conversation; clicking one hands the neighbouring prompt's row
 * key to the scroller, which owns the scroll itself.
 */
export const UserMessageJumpArrows: Story = {
	parameters: { messageScrollerHeight: 320 },
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "First prompt" }],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [
					{
						type: "text",
						text: "a".repeat(800),
					},
				],
			},
			{
				...baseMessage,
				id: 3,
				role: "user",
				content: [{ type: "text", text: "Second prompt" }],
			},
			{
				...baseMessage,
				id: 4,
				role: "assistant",
				content: [
					{
						type: "text",
						text: "b".repeat(800),
					},
				],
			},
			{
				...baseMessage,
				id: 5,
				role: "user",
				content: [{ type: "text", text: "Third prompt" }],
			},
		]),
		onEditUserMessage: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Reveal the hover-only action rows so we can interact with
		// the chevron buttons without dispatching real hover events.
		for (const el of canvasElement.querySelectorAll("[class]")) {
			if (
				el instanceof HTMLElement &&
				el.className.includes("group-hover/msg:opacity-100")
			) {
				el.style.opacity = "1";
			}
		}

		const prevButtons = canvas.getAllByRole("button", {
			name: "Jump to previous user message",
		});
		const nextButtons = canvas.getAllByRole("button", {
			name: "Jump to next user message",
		});
		expect(prevButtons).toHaveLength(3);
		expect(nextButtons).toHaveLength(3);

		// First user prompt: previous disabled, next enabled.
		expect(prevButtons[0]).toBeDisabled();
		expect(nextButtons[0]).toBeEnabled();

		// Middle user prompt: both directions enabled.
		expect(prevButtons[1]).toBeEnabled();
		expect(nextButtons[1]).toBeEnabled();

		// Last user prompt: previous enabled, next disabled.
		expect(prevButtons[2]).toBeEnabled();
		expect(nextButtons[2]).toBeDisabled();

		await userEvent.click(nextButtons[0]);
	},
};

/** Copy + edit actions appear below user messages on hover. */
export const UserMessageCopyButton: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "Can you fix this bug?" }],
			},
		]),
		onEditUserMessage: fn(),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		// Force the hover-reveal toolbar visible for the screenshot.
		for (const el of canvasElement.querySelectorAll("[class]")) {
			if (
				el instanceof HTMLElement &&
				el.className.includes("group-hover/msg:opacity-100")
			) {
				el.style.opacity = "1";
			}
		}
		const copyButton = canvas.getByRole("button", {
			name: "Copy message",
		});
		expect(copyButton).toBeInTheDocument();
		const editButton = canvas.getByRole("button", {
			name: "Edit message",
		});
		expect(editButton).toBeInTheDocument();

		// Behavioral: clicking edit fires onEditUserMessage with the
		// correct message ID and text.
		await userEvent.click(editButton);
		expect(args.onEditUserMessage).toHaveBeenCalledWith(
			1,
			"Can you fix this bug?",
			undefined,
		);

		// Behavioral: clicking copy writes the raw markdown to the
		// clipboard.
		const originalClipboard = navigator.clipboard;
		const writeText = fn().mockResolvedValue(undefined);
		Object.defineProperty(navigator, "clipboard", {
			value: { writeText },
			writable: true,
			configurable: true,
		});
		try {
			await userEvent.click(copyButton);
			expect(writeText).toHaveBeenCalledWith("Can you fix this bug?");
		} finally {
			Object.defineProperty(navigator, "clipboard", {
				value: originalClipboard,
				writable: true,
				configurable: true,
			});
		}
	},
};

/** Copy button is present on assistant messages on hover. */
export const AssistantMessageCopyButton: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "Explain this code" }],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [
					{
						type: "text",
						text: "This function handles **authentication** by checking the JWT token.\n\n```go\nfunc auth(r *http.Request) error {\n\treturn nil\n}\n```",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Force the hover-reveal toolbar visible.
		for (const el of canvasElement.querySelectorAll("[class]")) {
			if (
				el instanceof HTMLElement &&
				el.className.includes("group-hover/msg:opacity-100")
			) {
				el.style.opacity = "1";
			}
		}
		const actions = canvas.getAllByTestId("message-actions");
		expect(actions.length).toBeGreaterThanOrEqual(1);
		// The last message-actions belongs to the assistant.
		const assistantActions = actions[actions.length - 1];
		const copyBtn = within(assistantActions).getByRole("button", {
			name: "Copy message",
		});
		expect(copyBtn).toBeInTheDocument();
	},
};

/** Persisted ask-user-question answers survive reloads. */
export const AskUserQuestionSubmittedAnswer: Story = {
	args: {
		...defaultArgs,
		isChatCompleted: true,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "Help me pick a rollout plan." }],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [
					{
						type: "tool-call",
						tool_call_id: "ask-tool-1",
						tool_name: "ask_user_question",
					},
				],
			},
			{
				...baseMessage,
				id: 3,
				role: "tool",
				content: [
					{
						type: "tool-result",
						tool_call_id: "ask-tool-1",
						result: {
							output: JSON.stringify(askUserQuestionPayload),
						},
					},
				],
			},
			{
				...baseMessage,
				id: 4,
				role: "user",
				content: [{ type: "text", text: askUserQuestionSubmittedResponse }],
			},
		]),
	},
};

/** Copy button calls clipboard API with the raw markdown text. */
export const CopyButtonWritesToClipboard: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "What is the answer?" }],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [{ type: "text", text: "Here is the **answer**." }],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const originalClipboard = navigator.clipboard;
		const writeText = fn().mockResolvedValue(undefined);
		Object.defineProperty(navigator, "clipboard", {
			value: { writeText },
			writable: true,
			configurable: true,
		});

		try {
			const canvas = within(canvasElement);
			// Force the hover-reveal toolbar visible.
			for (const el of canvasElement.querySelectorAll("[class]")) {
				if (
					el instanceof HTMLElement &&
					el.className.includes("group-hover/msg:opacity-100")
				) {
					el.style.opacity = "1";
				}
			}
			// Find the assistant's copy button (last message-actions).
			const actions = canvas.getAllByTestId("message-actions");
			const assistantActions = actions[actions.length - 1];
			const copyBtn = within(assistantActions).getByRole("button", {
				name: "Copy message",
			});
			await userEvent.click(copyBtn);
			expect(writeText).toHaveBeenCalledWith("Here is the **answer**.");
		} finally {
			Object.defineProperty(navigator, "clipboard", {
				value: originalClipboard,
				writable: true,
				configurable: true,
			});
		}
	},
};

/**
 * Regression: sources-only assistant messages must have consistent
 * bottom spacing before the next user bubble. A spacer div fills the
 * gap that would normally come from the hidden action bar.
 */
export const SourcesOnlyAssistantSpacing: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "Can you share your sources?" }],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [
					{
						type: "source",
						url: "https://example.com/docs",
						title: "Documentation",
					},
					{
						type: "source",
						url: "https://example.com/api",
						title: "API Reference",
					},
				],
			},
			{
				...baseMessage,
				id: 3,
				role: "user",
				content: [{ type: "text", text: "Thanks!" }],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Can you share your sources?")).toBeInTheDocument();
		expect(canvas.getByText("Thanks!")).toBeInTheDocument();
		await userEvent.click(
			canvas.getByRole("button", { name: /searched 2 results/i }),
		);
		expect(
			canvas.getByRole("link", { name: "Documentation" }),
		).toBeInTheDocument();
		expect(
			canvas.getByRole("link", { name: "API Reference" }),
		).toBeInTheDocument();
	},
};

/**
 * Regression: action bar must appear on the last *visible* assistant
 * message even when invisible assistant messages (provider-executed
 * tool-result-only) follow it before the next user turn.
 */
export const AssistantActionBarAfterHiddenMessages: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "Help me refactor" }],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [
					{ type: "text", text: "Here is the **refactored** version." },
				],
			},
			{
				...baseMessage,
				id: 3,
				role: "assistant",
				content: [
					{
						type: "tool-result",
						tool_call_id: "provider-tool-1",
						result: { output: "done" },
						provider_executed: true,
					},
				],
			},
			{
				...baseMessage,
				id: 4,
				role: "user",
				content: [{ type: "text", text: "Thanks!" }],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Force the hover-reveal action bars visible using stable test IDs.
		for (const el of canvasElement.querySelectorAll(
			'[data-testid="message-actions"]',
		)) {
			if (el instanceof HTMLElement) {
				el.style.opacity = "1";
			}
		}
		// 2 user messages + 1 visible assistant = 3 action bars.
		// The invisible provider-executed tool-result message (id=3)
		// must not prevent the assistant (id=2) from showing its bar.
		const actions = canvas.getAllByTestId("message-actions");
		expect(actions).toHaveLength(3);
	},
};

export const ToolDisplayModesFromPreferences: Story = {
	parameters: {
		queries: [
			{
				key: ["me", "preferences"],
				data: {
					task_notification_alert_dismissed: false,
					thinking_display_mode: "auto" as const,
					shell_tool_display_mode: "always_collapsed" as const,
					code_diff_display_mode: "always_collapsed" as const,
					agent_chat_send_shortcut: "enter" as const,
				},
			},
		],
	},
	args: {
		...defaultArgs,
		parsedMessages: [
			{
				message: {
					...baseMessage,
					id: 1,
					role: "assistant",
					content: [],
				},
				parsed: {
					markdown: "",
					reasoning: "",
					toolCalls: [],
					toolResults: [],
					tools: [
						{
							id: "execute-tool",
							name: "execute",
							args: { command: "pnpm test" },
							result: { output: "tests passed" },
							isError: false,
							status: "completed",
						},
						{
							id: "edit-tool",
							name: "edit_files",
							args: {
								files: [
									{
										path: "src/config.ts",
										edits: [
											{
												search: "const timeout = 30;",
												replace: "const timeout = 60;",
											},
										],
									},
								],
							},
							result: { ok: true },
							isError: false,
							status: "completed",
						},
					],
					blocks: [
						{ type: "tool", id: "execute-tool" },
						{ type: "tool", id: "edit-tool" },
					],
					sources: [],
					hookNotices: [],
				},
			},
		] satisfies ParsedMessageEntry[],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const commandOutputButton = canvas.getByRole("button", {
			name: "Expand command",
		});
		expect(commandOutputButton).toHaveTextContent("Ran pnpm test");
		expect(canvas.queryByText("tests passed")).not.toBeInTheDocument();
		expect(canvas.getByText(/Edited config\.ts/)).toBeVisible();
		expect(canvas.queryAllByTestId("edit-file-diff")).toHaveLength(0);
		expect(commandOutputButton).toHaveAttribute("aria-expanded", "false");
		await userEvent.click(commandOutputButton);
		await waitFor(() => {
			expect(canvas.getByText("tests passed")).toBeVisible();
		});

		const editFilesButton = canvas.getByRole("button", {
			name: /Edited config\.ts/,
		});
		expect(editFilesButton).toHaveAttribute("aria-expanded", "false");
		await userEvent.click(editFilesButton);
		await waitFor(() => {
			expect(canvas.getAllByTestId("edit-file-diff")).toHaveLength(1);
		});
	},
};

/**
 * A completed thinking block with always_expanded mode should show
 * its content without user interaction.
 */
export const ThinkingBlockAlwaysExpanded: Story = {
	parameters: {
		queries: [
			{
				key: ["me", "preferences"],
				data: {
					task_notification_alert_dismissed: false,
					thinking_display_mode: "always_expanded" as const,
					shell_tool_display_mode: "auto" as const,
					code_diff_display_mode: "auto" as const,
					agent_chat_send_shortcut: "enter" as const,
				},
			},
		],
	},
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "assistant",
				content: [
					{
						type: "reasoning",
						text: "**Configuring model settings**\n\nLet me think about this step by step.",
					},
					{
						type: "text",
						text: "Here is the answer.",
					},
				],
			},
		]),
	},
};

/**
 * A completed thinking block with always_collapsed mode should
 * hide its content until the user clicks.
 */
export const ThinkingBlockAlwaysCollapsed: Story = {
	parameters: {
		queries: [
			{
				key: ["me", "preferences"],
				data: {
					task_notification_alert_dismissed: false,
					thinking_display_mode: "always_collapsed" as const,
					shell_tool_display_mode: "auto" as const,
					code_diff_display_mode: "auto" as const,
					agent_chat_send_shortcut: "enter" as const,
				},
			},
		],
	},
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "assistant",
				content: [
					{
						type: "reasoning",
						text: "Let me think about this step by step.",
					},
					{
						type: "text",
						text: "Here is the answer.",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Thinking")).toBeInTheDocument();
		expect(
			canvas.queryByText(/Let me think about this step by step/),
		).not.toBeInTheDocument();
		await userEvent.click(canvas.getByText("Thinking"));
		await waitFor(() => {
			expect(
				canvas.getByText(/Let me think about this step by step/),
			).toBeVisible();
		});
	},
};

export const SequentialReadFilesCollapsed: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "assistant",
				content: [{ type: "text", text: "I'll inspect the relevant files." }],
			},
			...buildReadFileExchange(
				2,
				"read-1",
				"site/src/a.ts",
				"export const a = 1;",
			),
			...buildReadFileExchange(
				4,
				"read-2",
				"site/src/b.ts",
				"export const b = 2;",
			),
			...buildReadFileExchange(
				6,
				"read-3",
				"site/src/c.ts",
				"export const c = 3;",
			),
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const groupButton = canvas.getByRole("button", { name: /read 3 files/i });
		expect(groupButton).toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: /read a\.ts/i }),
		).not.toBeInTheDocument();
		await userEvent.click(groupButton);
		await waitFor(() => {
			expect(canvas.getByRole("button", { name: /read a\.ts/i })).toBeVisible();
			expect(canvas.getByRole("button", { name: /read b\.ts/i })).toBeVisible();
			expect(canvas.getByRole("button", { name: /read c\.ts/i })).toBeVisible();
		});
		const firstFileButton = canvas.getByRole("button", { name: /read a\.ts/i });
		expect(firstFileButton).toHaveAttribute("aria-expanded", "false");

		await userEvent.click(firstFileButton);
		await waitFor(() => {
			expect(firstFileButton).toHaveAttribute("aria-expanded", "true");
		});
	},
};

export const ReadFileRewrittenByHook: Story = {
	args: {
		...defaultArgs,
		parsedMessages: [
			buildParsedReadFileEntry({
				messageId: 1,
				toolId: "read-rewritten-1",
				path: "site/src/redacted.ts",
				status: "completed",
				content: "export const redacted = true;\n",
				hookRewritten: true,
			}),
		],
	},
};

export const GroupedReadFilesRewrittenByHook: Story = {
	args: {
		...defaultArgs,
		parsedMessages: [
			buildParsedReadFileEntry({
				messageId: 1,
				toolId: "read-grouped-1",
				path: "site/src/a.ts",
				status: "completed",
				content: "export const a = 1;\n",
			}),
			buildParsedReadFileEntry({
				messageId: 2,
				toolId: "read-grouped-2",
				path: "site/src/b.ts",
				status: "completed",
				content: "export const b = 2;\n",
				hookRewritten: true,
			}),
		],
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);
		await step("group header shows the aggregate badge", async () => {
			expect(await canvas.findByText("Modified by policy")).toBeVisible();
		});
		await step("expanded rows credit only the rewritten file", async () => {
			await userEvent.click(
				await canvas.findByRole("button", { name: /Read 2 files/ }),
			);
			expect(
				await canvas.findByRole("button", { name: /Read b\.ts/ }),
			).toBeVisible();
			const attributed = canvas
				.getAllByRole("group", { name: "Modified by policy" })
				.map((group) => group.textContent ?? "");
			expect(attributed.some((text) => text.includes("b.ts"))).toBe(true);
			expect(attributed.some((text) => text.includes("a.ts"))).toBe(false);
			expect(canvas.getAllByText("Modified by policy")).toHaveLength(2);
		});
	},
};

export const SequentialReadFilesEmptyAndErrorStates: Story = {
	args: {
		...defaultArgs,
		parsedMessages: [
			buildParsedReadFileEntry({
				messageId: 1,
				toolId: "read-empty-1",
				path: "site/src/empty-a.ts",
				status: "completed",
			}),
			buildParsedReadFileEntry({
				messageId: 2,
				toolId: "read-empty-2",
				path: "site/src/empty-b.ts",
				status: "completed",
			}),
			...buildMessages([
				{
					...baseMessage,
					id: 3,
					role: "assistant",
					content: [{ type: "text", text: "Trying a different file set." }],
				},
			]),
			buildParsedReadFileEntry({
				messageId: 4,
				toolId: "read-error-1",
				path: "site/src/missing-a.ts",
				status: "error",
				errorMessage: "ENOENT: no such file or directory",
			}),
			buildParsedReadFileEntry({
				messageId: 5,
				toolId: "read-error-2",
				path: "site/src/missing-b.ts",
				status: "error",
				errorMessage: "permission denied",
			}),
		] satisfies ParsedMessageEntry[],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const buttons = canvas.getAllByRole("button", { name: /read 2 files/i });
		expect(buttons).toHaveLength(2);

		await userEvent.click(buttons[0]);
		await waitFor(() => {
			expect(canvas.getByText("Read empty-a.ts")).toBeVisible();
			expect(canvas.getByText("Read empty-b.ts")).toBeVisible();
		});

		await userEvent.click(buttons[1]);
		await waitFor(() => {
			expect(
				canvas.getByRole("button", { name: /read missing-a\.ts/i }),
			).toBeVisible();
			expect(
				canvas.getByRole("button", { name: /read missing-b\.ts/i }),
			).toBeVisible();
		});

		await userEvent.click(
			canvas.getByRole("button", { name: /read missing-a\.ts/i }),
		);
		await waitFor(() => {
			expect(
				canvas.getByText("ENOENT: no such file or directory"),
			).toBeVisible();
		});
	},
};

export const SequentialReadFilesRunningState: Story = {
	args: {
		...defaultArgs,
		parsedMessages: [
			buildParsedReadFileEntry({
				messageId: 1,
				toolId: "read-running-1",
				path: "site/src/one.ts",
				status: "running",
			}),
			buildParsedReadFileEntry({
				messageId: 2,
				toolId: "read-running-2",
				path: "site/src/two.ts",
				status: "running",
			}),
			buildParsedReadFileEntry({
				messageId: 3,
				toolId: "read-running-3",
				path: "site/src/three.ts",
				status: "running",
			}),
		] satisfies ParsedMessageEntry[],
	},
};

/** Collapsed thinking should visually align with adjacent tool calls. */
export const ThinkingBlockWithToolCall: Story = {
	parameters: {
		queries: [
			{
				key: ["me", "preferences"],
				data: {
					task_notification_alert_dismissed: false,
					thinking_display_mode: "always_collapsed" as const,
					shell_tool_display_mode: "auto" as const,
					code_diff_display_mode: "auto" as const,
					agent_chat_send_shortcut: "enter" as const,
				},
			},
		],
	},
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "assistant",
				content: [
					{
						type: "reasoning",
						text: "I need to inspect the package metadata before answering.",
					},
					{
						type: "tool-call",
						tool_call_id: "tool-1",
						tool_name: "read_file",
						args: { path: "package.json" },
					},
				],
			},
			{
				...baseMessage,
				id: 2,
				role: "tool",
				content: [
					{
						type: "tool-result",
						tool_call_id: "tool-1",
						result: { content: '{"name":"coder"}' },
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const thinkingButton = canvas.getByRole("button", { name: /thinking/i });
		expect(thinkingButton).toBeInTheDocument();
		expect(
			canvas.getByRole("button", { name: /read package\.json/i }),
		).toBeInTheDocument();

		const toolButton = canvas.getByRole("button", {
			name: /read package\.json/i,
		});
		const thinkingContainer =
			thinkingButton.closest("[data-transcript-row]") ?? thinkingButton;
		const toolContainer =
			toolButton.closest("[data-transcript-row]") ?? toolButton;
		expect(
			toolContainer.firstElementChild ?? toolContainer,
		).not.toHaveAttribute("data-state");
		expect(
			thinkingContainer.firstElementChild ?? thinkingContainer,
		).not.toHaveAttribute("data-state");
		expect(
			canvas.queryByTestId("assistant-bottom-spacer"),
		).not.toBeInTheDocument();
	},
};

/** Shell-style tool rows should keep the same collapsed height as Thinking. */
export const ThinkingBlockWithShellTools: Story = {
	parameters: {
		queries: [
			{
				key: ["me", "preferences"],
				data: {
					task_notification_alert_dismissed: false,
					thinking_display_mode: "always_collapsed" as const,
					shell_tool_display_mode: "always_collapsed" as const,
					code_diff_display_mode: "auto" as const,
					agent_chat_send_shortcut: "enter" as const,
				},
			},
		],
	},
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "assistant",
				content: [
					{
						type: "reasoning",
						text: "I should inspect the current chat spacing before patching it.",
					},
					{
						type: "tool-call",
						tool_call_id: "tool-1",
						tool_name: "execute",
						args: { command: "pnpm test" },
					},
					{
						type: "tool-call",
						tool_call_id: "tool-2",
						tool_name: "process_output",
						args: { process_id: "process-1" },
					},
				],
			},
			{
				...baseMessage,
				id: 2,
				role: "tool",
				content: [
					{
						type: "tool-result",
						tool_call_id: "tool-1",
						tool_name: "execute",
						result: { output: "", wall_duration_ms: "667" },
					},
				],
			},
			{
				...baseMessage,
				id: 3,
				role: "tool",
				content: [
					{
						type: "tool-result",
						tool_call_id: "tool-2",
						tool_name: "process_output",
						result: { output: "Spacing looks stable." },
					},
				],
			},
		]),
	},
};
