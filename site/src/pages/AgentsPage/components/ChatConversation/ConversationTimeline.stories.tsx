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

/** Copy + edit toolbar appears below user messages on hover. */
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

/** Copy button is present on assistant messages below the response. */
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
		// The assistant copy button is always visible below
		// the response content.
		const wrapper = canvas.getByTestId("assistant-copy-button");
		const copyBtn = within(wrapper).getByRole("button", {
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
		// Tool-only assistant message should not have a copy button.
		expect(
			canvas.queryByTestId("assistant-copy-button"),
		).not.toBeInTheDocument();
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
			// Find the always-visible assistant copy button.
			const wrapper = canvas.getByTestId("assistant-copy-button");
			const copyBtn = within(wrapper).getByRole("button", {
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
 * Regression: copy button appears only on the last assistant message
 * in a turn that includes tool calls. The isLastAssistantMessage
 * computation must skip tool-role messages when finding turn
 * boundaries.
 */
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
		// Only the last assistant message in the turn should have the
		// copy button. The first assistant message (id=2) has text but
		// should not show the button because a later assistant message
		// (id=4) continues the turn.
		const wrappers = canvas.getAllByTestId("assistant-copy-button");
		expect(wrappers).toHaveLength(1);

		const copyBtn = within(wrappers[0]).getByRole("button", {
			name: "Copy message",
		});
		expect(copyBtn).toBeInTheDocument();
	},
};
