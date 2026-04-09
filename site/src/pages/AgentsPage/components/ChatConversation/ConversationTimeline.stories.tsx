import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, spyOn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { ConversationTimeline } from "./ConversationTimeline";
import { parseMessagesWithMergedTools } from "./messageParsing";

// 1×1 solid coral (#FF6B6B) PNG encoded as base64.
const TEST_PNG_B64 =
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg==";

const buildMessages = (messages: TypesGen.ChatMessage[]) =>
	parseMessagesWithMergedTools(messages);

const baseMessage = {
	chat_id: "story-chat",
	created_at: "2026-03-10T00:00:00.000Z",
} as const;

const TEXT_ATTACHMENT_RESPONSES = new Map<string, string>([
	[
		"storybook-test-text",
		"Quarterly revenue increased 18% year over year after the new pricing rollout stabilized customer expansion.",
	],
	[
		"storybook-text-only",
		"Runbook note: restart the worker after updating the queue configuration to pick up the new concurrency limits.",
	],
	[
		"storybook-text-1",
		"First context file: deployment checklist and rollback instructions for the release candidate.",
	],
	[
		"storybook-text-2",
		"Second context file: service logs showing a transient timeout while the cache warmed up.",
	],
	[
		"storybook-text-3",
		"Third context file: local development configuration overrides for reproducing the issue.",
	],
]);

const mockTextAttachmentFetch = () => {
	const originalFetch = globalThis.fetch;
	spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
		const url =
			typeof input === "string"
				? input
				: input instanceof URL
					? input.toString()
					: input.url;

		for (const [fileId, content] of TEXT_ATTACHMENT_RESPONSES) {
			if (url.endsWith(fileId)) {
				return new Response(content, { status: 200 });
			}
		}

		return originalFetch(input, init);
	});
};

const defaultArgs: Omit<
	React.ComponentProps<typeof ConversationTimeline>,
	"parsedMessages"
> = {
	subagentTitles: new Map(),
};

const meta: Meta<typeof ConversationTimeline> = {
	title: "pages/AgentsPage/ChatConversation/ConversationTimeline",
	component: ConversationTimeline,
	decorators: [
		(Story) => (
			<div className="mx-auto w-full max-w-3xl py-6">
				<Story />
			</div>
		),
	],
	beforeEach: () => {
		mockTextAttachmentFetch();
	},
};
export default meta;
type Story = StoryObj<typeof ConversationTimeline>;

const TIMELINE_SCROLL_CONTAINER_TEST_ID = "timeline-scroll-container";

const renderInScrollableTimeline = (
	args: React.ComponentProps<typeof ConversationTimeline>,
) => (
	<div
		data-testid={TIMELINE_SCROLL_CONTAINER_TEST_ID}
		className="h-[600px] overflow-y-auto rounded-md border border-border-default bg-surface-primary p-4"
	>
		<ConversationTimeline {...args} />
	</div>
);

const findTimelineScroller = (canvasElement: HTMLElement): HTMLElement => {
	const scroller = canvasElement.querySelector(
		`[data-testid='${TIMELINE_SCROLL_CONTAINER_TEST_ID}']`,
	);
	if (!(scroller instanceof HTMLElement)) {
		throw new Error("Timeline scroll container not found");
	}
	return scroller;
};

const waitForAnimationFrame = () =>
	new Promise<void>((resolve) => {
		requestAnimationFrame(() => resolve());
	});

const makeChatMessage = (
	id: number,
	role: TypesGen.ChatMessage["role"],
	content: readonly TypesGen.ChatMessagePart[],
): TypesGen.ChatMessage => ({
	...baseMessage,
	id,
	role,
	created_at: new Date(
		Date.parse(baseMessage.created_at) + id * 60_000,
	).toISOString(),
	content,
});

const buildLargeHistoryMessages = (count: number): TypesGen.ChatMessage[] =>
	Array.from({ length: count }, (_, index) => {
		const id = index + 1;
		const role = id % 2 === 0 ? "assistant" : "user";
		if (role === "user") {
			return makeChatMessage(id, role, [
				{
					type: "text",
					text: `User prompt #${id}: Please summarize the latest build output.`,
				},
			]);
		}

		const isLongResponse = id % 6 === 0;
		return makeChatMessage(id, role, [
			{
				type: "text",
				text: isLongResponse
					? `### Assistant response #${id}\n\nI reviewed the workspace logs and captured the most relevant signals for this step.\n\n- build status: completed\n- tests: 147 passing\n- warnings: 2 deprecations to follow up\n\nNext, I can open a focused diff to verify the warning sources.`
					: `Assistant response #${id}: quick confirmation that this step completed.`,
			},
		]);
	});

const buildStickyFollowupMessages = (
	assistantFollowups: number,
): TypesGen.ChatMessage[] => {
	const messages: TypesGen.ChatMessage[] = [
		makeChatMessage(1, "user", [
			{
				type: "text",
				text: "Investigate why the canary deployment rolled back in staging.",
			},
		]),
	];

	for (let step = 1; step <= assistantFollowups; step++) {
		const toolCallID = `sticky-tool-${step}`;
		messages.push(
			makeChatMessage(step + 1, "assistant", [
				{
					type: "reasoning",
					text: `Checking telemetry segment ${step} for rollback indicators.`,
				},
				{
					type: "tool-call",
					tool_call_id: toolCallID,
					tool_name: "read_file",
					args: { path: `logs/segment-${step}.txt` },
				},
				{
					type: "tool-result",
					tool_call_id: toolCallID,
					tool_name: "read_file",
					result: { output: `segment-${step}: inspected successfully` },
				},
				{
					type: "text",
					text: `Follow-up ${step}: parsed logs and queued the next diagnostic check.`,
				},
			]),
		);
	}

	return messages;
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.getAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(1);
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.getAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(3);
	},
};

/** File-id images use a server URL instead of inline base64 data. */
export const UserMessageWithFileIdImage: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Uploaded via file ID" },
					{
						type: "file",
						media_type: "image/png",
						file_id: "storybook-test-image",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.getAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(1);
		// Verify file_id path is used, not a base64 data URI.
		expect(images[0]).toHaveAttribute(
			"src",
			"/api/experimental/chats/files/storybook-test-image",
		);
	},
};

export const UserMessageWithTextAttachment: Story = {
	args: {
		...defaultArgs,
		parsedMessages: parseMessagesWithMergedTools([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Here is some context from our docs:" },
					{
						type: "file",
						file_id: "storybook-test-text",
						media_type: "text/plain",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const textButton = await canvas.findByRole("button", {
			name: "View text attachment",
		});
		expect(textButton).toBeInTheDocument();
		expect(textButton).toHaveTextContent(/Pasted text/i);
		await userEvent.click(textButton);
		expect(
			await canvas.findByText(/Quarterly revenue increased 18%/i),
		).toBeInTheDocument();
	},
};

export const UserMessageWithMultipleTextAttachments: Story = {
	args: {
		...defaultArgs,
		parsedMessages: parseMessagesWithMergedTools([
			{
				...baseMessage,
				id: 1,
				created_at: "2025-01-15T10:00:00Z",
				role: "user",
				content: [
					{ type: "text", text: "Here are several context files:" },
					{
						type: "file",
						file_id: "storybook-text-1",
						media_type: "text/plain",
					},
					{
						type: "file",
						file_id: "storybook-text-2",
						media_type: "text/plain",
					},
					{
						type: "file",
						file_id: "storybook-text-3",
						media_type: "text/plain",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const textButtons = await canvas.findAllByRole("button", {
			name: "View text attachment",
		});
		expect(textButtons).toHaveLength(3);
	},
};

export const UserMessageWithTextAttachmentOnly: Story = {
	args: {
		...defaultArgs,
		parsedMessages: parseMessagesWithMergedTools([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{
						type: "file",
						file_id: "storybook-text-only",
						media_type: "text/plain",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const textButton = await canvas.findByRole("button", {
			name: "View text attachment",
		});
		expect(textButton).toHaveTextContent(/Pasted text/i);
		await userEvent.click(textButton);
		expect(
			await canvas.findByText(/Runbook note: restart the worker/i),
		).toBeInTheDocument();
	},
};

/** Visual regression: text and image attachments render at the same height. */
export const UserMessageWithMixedAttachments: Story = {
	args: {
		...defaultArgs,
		parsedMessages: parseMessagesWithMergedTools([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Here is a screenshot and some context" },
					{
						type: "file",
						media_type: "image/png",
						data: TEST_PNG_B64,
					},
					{
						type: "file",
						file_id: "storybook-test-text",
						media_type: "text/plain",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.getAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(1);
		const textButtons = await canvas.findAllByRole("button", {
			name: "View text attachment",
		});
		expect(textButtons).toHaveLength(1);
	},
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.queryAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(0);
		expect(canvas.getByText("Just a plain text message")).toBeInTheDocument();
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
				role: "user",
				content: [{ type: "text", text: "Generate an image" }],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [
					{ type: "text", text: "Here is the generated image:" },
					{
						type: "file",
						media_type: "image/png",
						data: TEST_PNG_B64,
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.getAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(1);
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.getAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(1);
		expect(canvas.getByText(/main\.go/)).toBeInTheDocument();
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Button\.tsx/)).toBeInTheDocument();
		expect(canvas.getByText(/Can you refactor/)).toBeInTheDocument();
		expect(canvas.getByText(/to use the new API/)).toBeInTheDocument();
	},
};

/** Multiple file references render inline, no separate section. */
export const UserMessageWithMultipleInlineFileRefs: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Compare " },
					{
						type: "file-reference",
						file_name: "api/handler.go",
						start_line: 1,
						end_line: 50,
						content: "...",
					},
					{ type: "text", text: " with " },
					{
						type: "file-reference",
						file_name: "api/handler_test.go",
						start_line: 10,
						end_line: 30,
						content: "...",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/handler\.go/)).toBeInTheDocument();
		expect(canvas.getByText(/handler_test\.go/)).toBeInTheDocument();
	},
};

/**
 * Verifies the structural requirements for sticky user messages
 * in the flat (section-less) message list:
 * - Each user message renders a data-user-sentinel marker so
 *   the push-up logic can find the next user message via DOM
 *   traversal.
 * - The user message container gets position:sticky.
 * - Sentinels appear in the correct order (matching user
 *   message order).
 */
export const StickyUserMessageStructure: Story = {
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
	play: async ({ canvasElement }) => {
		// Each user message should produce a data-user-sentinel
		// marker that the push-up scroll logic relies on.
		const sentinels = canvasElement.querySelectorAll("[data-user-sentinel]");
		expect(sentinels.length).toBe(2);

		// Each sentinel should be immediately followed by a sticky
		// container (the user message itself).
		for (const sentinel of sentinels) {
			const container = sentinel.nextElementSibling;
			expect(container).not.toBeNull();
			const style = window.getComputedStyle(container!);
			expect(style.position).toBe("sticky");
		}

		// Sentinels must appear in DOM order matching the message
		// order so nextElementSibling traversal finds the correct
		// next user message.
		const allElements = Array.from(
			canvasElement.querySelectorAll("[data-user-sentinel], [class*='sticky']"),
		);
		const sentinelIndices = Array.from(sentinels).map((s) =>
			allElements.indexOf(s),
		);
		// Sentinels should be in ascending DOM order.
		expect(sentinelIndices[0]).toBeLessThan(sentinelIndices[1]);

		// Both user messages should be visible.
		const canvas = within(canvasElement);
		expect(canvas.getByText("First prompt")).toBeVisible();
		expect(canvas.getByText("Second prompt")).toBeVisible();
	},
};

const LARGE_HISTORY_MESSAGE_COUNT = 60;

export const LargeMessageHistory: Story = {
	render: renderInScrollableTimeline,
	args: {
		...defaultArgs,
		parsedMessages: buildMessages(
			buildLargeHistoryMessages(LARGE_HISTORY_MESSAGE_COUNT),
		),
	},
	play: async ({ canvasElement }) => {
		const userSentinels = canvasElement.querySelectorAll(
			"[data-user-sentinel]",
		);
		expect(userSentinels).toHaveLength(LARGE_HISTORY_MESSAGE_COUNT / 2);

		const assistantMessages = canvasElement.querySelectorAll(
			"[data-role='assistant']",
		);
		expect(assistantMessages).toHaveLength(LARGE_HISTORY_MESSAGE_COUNT / 2);
	},
};

export const MixedContentHeights: Story = {
	render: renderInScrollableTimeline,
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			makeChatMessage(1, "user", [
				{
					type: "text",
					text: "The timeline jumps during long responses. Can you investigate?",
				},
			]),
			makeChatMessage(2, "assistant", [
				{
					type: "reasoning",
					text: "I'll inspect the render path and compare block heights while scrolling.",
				},
				{
					type: "tool-call",
					tool_call_id: "mixed-tool-1",
					tool_name: "read_file",
					args: {
						path: "site/src/pages/AgentsPage/components/ChatConversation/ConversationTimeline.tsx",
					},
				},
			]),
			makeChatMessage(3, "tool", [
				{
					type: "tool-result",
					tool_call_id: "mixed-tool-1",
					tool_name: "read_file",
					result: {
						output:
							"Sticky overlay math uses --clip-h and updates on scroll + ResizeObserver events.",
					},
				},
			]),
			makeChatMessage(4, "assistant", [
				{
					type: "text",
					text: "### Height variance findings\n\nThe tallest regions come from markdown sections that include multiple paragraphs, list items, and inline code references.\n\nWhen these long blocks sit next to terse user prompts, the transcript alternates between very small and very large rows.\n\n- Tool cards add compact but non-trivial height\n- Reasoning blocks are denser than plain text\n- Attachment previews add fixed-height chunks",
				},
			]),
			makeChatMessage(5, "user", [
				{
					type: "text",
					text: "Can you check one migration reference too?",
				},
			]),
			makeChatMessage(6, "assistant", [
				{ type: "text", text: "I traced the reference in " },
				{
					type: "file-reference",
					file_name: "coderd/database/migrations/000320_add_chat_tables.up.sql",
					start_line: 1,
					end_line: 24,
					content: "CREATE TABLE chat_messages (...);",
				},
				{
					type: "text",
					text: " and it stays stable when replaying the same conversation data.",
				},
			]),
			makeChatMessage(7, "assistant", [
				{
					type: "text",
					text: "I also attached the runbook snippet with mitigation notes.",
				},
				{
					type: "file",
					file_id: "storybook-text-2",
					media_type: "text/plain",
				},
			]),
			makeChatMessage(8, "assistant", [
				{
					type: "tool-call",
					tool_call_id: "mixed-tool-2",
					tool_name: "grep",
					args: { path: "logs/deploy.log", pattern: "timeout" },
				},
				{
					type: "tool-result",
					tool_call_id: "mixed-tool-2",
					tool_name: "grep",
					result: { output: "timeout_count=3" },
				},
				{
					type: "text",
					text: "Tool scan found three timeout spikes immediately before each retry window.",
				},
			]),
			makeChatMessage(9, "assistant", [
				{
					type: "text",
					text: "Final assistant summary: mixed-height timeline reached the end cleanly.",
				},
			]),
		]),
	},
	play: async ({ canvasElement }) => {
		const scroller = findTimelineScroller(canvasElement);
		scroller.scrollTop = scroller.scrollHeight;
		scroller.dispatchEvent(new Event("scroll"));
		await waitForAnimationFrame();

		const canvas = within(canvasElement);
		expect(
			canvas.getByText(
				"Final assistant summary: mixed-height timeline reached the end cleanly.",
			),
		).toBeVisible();
	},
};

const STICKY_FOLLOWUP_COUNT = 12;

export const StickyUserMessageWithManyFollowups: Story = {
	render: renderInScrollableTimeline,
	args: {
		...defaultArgs,
		parsedMessages: buildMessages(
			buildStickyFollowupMessages(STICKY_FOLLOWUP_COUNT),
		),
	},
	play: async ({ canvasElement }) => {
		const scroller = findTimelineScroller(canvasElement);
		scroller.scrollTop = scroller.scrollHeight;
		scroller.dispatchEvent(new Event("scroll"));
		await waitForAnimationFrame();

		const firstSentinel = canvasElement.querySelector("[data-user-sentinel]");
		expect(firstSentinel).toBeInstanceOf(HTMLElement);
		const sentinel = firstSentinel as HTMLElement;

		const stickyContainer = sentinel.nextElementSibling;
		expect(stickyContainer).toBeInstanceOf(HTMLElement);
		const stickyMessage = stickyContainer as HTMLElement;
		expect(window.getComputedStyle(stickyMessage).position).toBe("sticky");

		const scrollerRect = scroller.getBoundingClientRect();
		const sentinelRect = sentinel.getBoundingClientRect();
		expect(sentinelRect.top).toBeLessThan(scrollerRect.top);

		const stickyRect = stickyMessage.getBoundingClientRect();
		expect(stickyRect.top).toBeGreaterThanOrEqual(scrollerRect.top - 1);
		expect(stickyRect.top).toBeLessThan(scrollerRect.top + 24);

		const canvas = within(canvasElement);
		expect(
			canvas.getByText(
				"Investigate why the canary deployment rolled back in staging.",
			),
		).toBeInTheDocument();
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

/** No copy button when assistant message has no markdown content. */
export const AssistantMessageNoCopyWhenToolOnly: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "Run the tests" }],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [
					{
						type: "tool-call",
						tool_call_id: "tool-1",
						tool_name: "execute",
						args: { command: "go test ./..." },
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
						tool_call_id: "tool-1",
						result: { output: "PASS" },
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
		// Only the user message should have actions; the tool-only
		// assistant message has no copyable content.
		const actions = canvas.getAllByTestId("message-actions");
		expect(actions).toHaveLength(1);
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

/** All messages get copy actions regardless of turn state. */
export const CopyButtonDuringActiveTurn: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "Fix the bug" }],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [{ type: "text", text: "Let me look at the code." }],
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
		// Both user and assistant messages should have actions.
		const actions = canvas.getAllByTestId("message-actions");
		expect(actions).toHaveLength(2);
	},
};

/** All assistant messages with text content get a copy button. */
export const MultiAssistantTurnCopyButton: Story = {
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
					{ type: "text", text: "Let me check the code first." },
					{
						type: "tool-call",
						tool_call_id: "tool-1",
						tool_name: "read_file",
						args: { path: "main.go" },
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
						tool_call_id: "tool-1",
						result: { output: "package main" },
					},
				],
			},
			{
				...baseMessage,
				id: 4,
				role: "assistant",
				content: [
					{ type: "text", text: "Here is the **refactored** version." },
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
		// The first assistant message (id=2) is mid-chain so its
		// actions are hidden. Only the user and the last assistant
		// (id=4) get action bars.
		const actions = canvas.getAllByTestId("message-actions");
		expect(actions).toHaveLength(2);
	},
};

/**
 * Regression: thinking-only assistant messages must have consistent
 * bottom spacing before the next user bubble. A spacer div fills the
 * gap that would normally come from the invisible action bar.
 */
export const ThinkingOnlyAssistantSpacing: Story = {
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
						type: "reasoning",
						text: "Let me think about this step by step. The user wants me to explain the code they shared.",
					},
				],
			},
			{
				...baseMessage,
				id: 3,
				role: "user",
				content: [{ type: "text", text: "Any progress?" }],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The thinking-only assistant message has no action bar, but
		// it should still have visible text and a spacer element.
		expect(canvas.getByText("Explain this code")).toBeInTheDocument();
		expect(canvas.getByText("Any progress?")).toBeInTheDocument();
	},
};
