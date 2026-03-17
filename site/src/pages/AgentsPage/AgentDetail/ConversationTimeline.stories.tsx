import type { Meta, StoryObj } from "@storybook/react-vite";
import type * as TypesGen from "api/typesGenerated";
import { createRef } from "react";
import { expect, fn, userEvent, within } from "storybook/test";
import { ConversationTimeline } from "./ConversationTimeline";
import {
	buildParsedMessageSections,
	parseMessagesWithMergedTools,
} from "./messageParsing";

// 1×1 solid coral (#FF6B6B) PNG encoded as base64.
const TEST_PNG_B64 =
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg==";

const buildSections = (messages: TypesGen.ChatMessage[]) =>
	buildParsedMessageSections(parseMessagesWithMergedTools(messages));

const baseMessage = {
	chat_id: "story-chat",
	created_at: "2026-03-10T00:00:00.000Z",
} as const;

const defaultArgs: Omit<
	React.ComponentProps<typeof ConversationTimeline>,
	"parsedSections"
> = {
	isEmpty: false,
	hasMoreMessages: false,
	loadMoreSentinelRef: createRef<HTMLDivElement>(),
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
		parsedSections: buildSections([
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
		parsedSections: buildSections([
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
		parsedSections: buildSections([
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
		parsedSections: buildSections([
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
		parsedSections: buildSections([
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
		parsedSections: buildSections([
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
						text: "main function",
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
		loadMoreSentinelRef: { current: null },
		parsedSections: [],
		detailError: {
			kind: "usage-limit",
			message:
				"You've used $50.00 of your $50.00 spend limit. Your limit resets on July 1, 2025.",
		},
		onOpenAnalytics: fn(),
		subagentTitles: new Map(),
		subagentStatusOverrides: new Map(),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/spend limit/i)).toBeVisible();
		const btn = canvas.getByRole("button", { name: /view usage/i });
		expect(btn).toBeVisible();
		await userEvent.click(btn);
		expect(args.onOpenAnalytics).toHaveBeenCalled();
	},
};

/** Non-usage errors must not show the usage CTA. */
export const GenericErrorDoesNotShowUsageAction: Story = {
	args: {
		...defaultArgs,
		loadMoreSentinelRef: { current: null },
		parsedSections: [],
		detailError: { kind: "generic", message: "Provider request failed." },
		onOpenAnalytics: fn(),
		subagentTitles: new Map(),
		subagentStatusOverrides: new Map(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/provider request failed/i)).toBeVisible();
		expect(
			canvas.queryByRole("button", { name: /view usage/i }),
		).not.toBeInTheDocument();
	},
};
