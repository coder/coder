import type { Meta, StoryObj } from "@storybook/react-vite";
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
import { getChatFileURL } from "../../utils/chatAttachments";
import { encodeInlineTextAttachment } from "../../utils/fetchTextAttachment";
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

const findAttachmentTile = async (
	canvas: ReturnType<typeof within>,
	label: string,
) => {
	const tile = await canvas.findByRole("img", { name: label });
	expect(canvas.getByText(label)).toBeInTheDocument();
	return tile;
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
		attachmentFetchCounts = new Map();
		mockAttachmentFetch();
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

		// The tooltip explains the retention policy generically so the
		// copy survives any operator-chosen retention window.
		await hoverAndExpectTooltip(
			expiredTile,
			/deleted after the retention window/i,
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
		await waitFor(() =>
			expect(
				canvas.getAllByRole("img", { name: "Image expired" }),
			).toHaveLength(2),
		);
		expect(getAttachmentFetchCount("storybook-expired-image")).toBe(1);
		expect(
			canvas.queryByRole("button", { name: "View Attached image" }),
		).not.toBeInTheDocument();
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
		await findAttachmentTile(canvas, "Image failed to load");
		expect(canvas.getByText("This image failed to load")).toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "View Attached image" }),
		).not.toBeInTheDocument();

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
		await findAttachmentTile(canvas, "Image failed to load");
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
		await findAttachmentTile(canvas, "Image failed to load");
		expect(
			canvas.getByText("Inline image data is corrupt"),
		).toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "View Attached image" }),
		).not.toBeInTheDocument();
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
			"/api/experimental/chats/files/storybook-user-deployment-report",
		);
		expect(canvas.getByText("deployment-report.pdf")).toBeInTheDocument();
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const textButtons = await canvas.findAllByRole("button", {
			name: "View text attachment",
		});
		expect(textButtons).toHaveLength(3);
	},
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
		await userEvent.click(textButton);
		expect(
			await canvas.findByText(/Runbook note: restart the worker/i),
		).toBeInTheDocument();
	},
};

export const UserMessageWithExpiredTextAttachment: Story = {
	args: buildStoryArgs(
		buildUserMessage({
			text: "This pasted context has expired",
			files: [buildTextAttachmentPart("storybook-expired-text")],
		}),
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const textButton = await canvas.findByRole("button", {
			name: "View text attachment",
		});
		await userEvent.click(textButton);
		const expiredTile = await findAttachmentTile(canvas, "Attachment expired");
		expect(
			canvas.getByText("This pasted context has expired"),
		).toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "View text attachment" }),
		).not.toBeInTheDocument();

		await hoverAndExpectTooltip(
			expiredTile,
			/deleted after the retention window/i,
		);
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

export const UserMessageWithInlineTextAttachment: Story = {
	args: {
		...defaultArgs,
		parsedMessages: parseMessagesWithMergedTools([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Here is inline context:" },
					{
						type: "file",
						media_type: "text/plain",
						data: encodeInlineTextAttachment(
							"Inline deployment note: verify the feature flag before rollout.",
						),
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
			await canvas.findByText(/Inline deployment note/i),
		).toBeInTheDocument();
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The submitted-answer summary is hidden after the follow-up user message.
		expect(
			canvas.getByText("How should we structure the database migration?"),
		).toBeInTheDocument();
		expect(canvas.queryAllByRole("radio")).toHaveLength(0);
		expect(
			canvas.queryByRole("button", { name: "Submit" }),
		).not.toBeInTheDocument();
		const userMessages = canvasElement.querySelectorAll('[data-role="user"]');
		const latestUserMessage = userMessages[userMessages.length - 1];
		if (!(latestUserMessage instanceof HTMLElement)) {
			throw new Error("Expected a submitted user message bubble.");
		}
		expect(
			within(latestUserMessage).getByText(
				/Implementation Approach: Incremental migrations/,
			),
		).toBeInTheDocument();
		expect(
			within(latestUserMessage).getByText(/Release Plan: Small beta/),
		).toBeInTheDocument();
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

export const NoRenderableContentFallbackSpacing: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 101,
				role: "assistant",
				content: [],
			},
			{
				...baseMessage,
				id: 102,
				role: "user",
				content: [{ type: "text", text: "Thanks for trying!" }],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText("Message has no renderable content."),
		).toBeInTheDocument();
		expect(
			document.querySelector('[data-testid="assistant-bottom-spacer"]'),
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
