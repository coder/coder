import type { Meta, StoryObj } from "@storybook/react-vite";
import type * as TypesGen from "api/typesGenerated";
import { expect, within } from "storybook/test";
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

const defaultArgs: Omit<
	React.ComponentProps<typeof ConversationTimeline>,
	"parsedMessages"
> = {
	isEmpty: false,
	hasStreamOutput: false,
	streamState: null,
	streamTools: [],
	subagentTitles: new Map(),
	subagentStatusOverrides: new Map(),
	isAwaitingFirstStreamChunk: false,
};

const meta: Meta<typeof ConversationTimeline> = {
	title: "pages/AgentsPage/AgentDetail/ConversationTimeline",
	component: ConversationTimeline,
	decorators: [
		(Story) => (
			<div className="mx-auto w-full max-w-3xl py-6">
				<Story />
			</div>
		),
	],
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

/** Assistant-side images go through renderBlockList, not the user path. */
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

/** Usage-limit errors render as an info alert with analytics access. */
export const UsageLimitExceeded: Story = {
	args: {
		...defaultArgs,
		parsedMessages: [],
		detailError: {
			kind: "usage-limit",
			message:
				"You've used $50.00 of your $50.00 spend limit. Your limit resets on July 1, 2025.",
		},

		subagentTitles: new Map(),
		subagentStatusOverrides: new Map(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/spend limit/i)).toBeVisible();
		const link = canvas.getByRole("link", { name: /view usage/i });
		expect(link).toBeVisible();
		expect(link).toHaveAttribute("href", "/agents/analytics");
	},
};

/** Non-usage errors must not show the usage CTA. */
export const GenericErrorDoesNotShowUsageAction: Story = {
	args: {
		...defaultArgs,
		parsedMessages: [],
		detailError: { kind: "generic", message: "Provider request failed." },
		subagentTitles: new Map(),
		subagentStatusOverrides: new Map(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/provider request failed/i)).toBeVisible();
		expect(
			canvas.queryByRole("link", { name: /view usage/i }),
		).not.toBeInTheDocument();
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
