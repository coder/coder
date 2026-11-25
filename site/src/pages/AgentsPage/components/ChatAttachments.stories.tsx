import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { mockApiError } from "#/testHelpers/entities";
import { ChatAttachments } from "./ChatAttachments";
import { getMessageAttachments } from "./ChatConversation/messageHelpers";

// A 120x80 bar chart, inline so the thumbnail loads without a server.
const mockDashboardScreenshotPng =
	"iVBORw0KGgoAAAANSUhEUgAAAHgAAABQBAMAAAAgtay6AAAAFVBMVEX7vySni/o005lgpfr4cXE4QlYYHiqCUmhAAAAAYklEQVR42mMMZSAfMDGMah7VPDw1swiOBthQiSo8crMgVNpogI1qHsJp+yyEMh4NsAErSbCC1RAqdDS0qRHafw5AxB1GA2zk5ap/DyA2KYwG2KjmUc2jmoePZsa00QAbIpoBDsAId7SA4AAAAAAASUVORK5CYII=";

const mockAttachmentMessage: TypesGen.ChatMessage = {
	...MockChatMessage,
	role: "assistant",
	content: [
		{
			type: "file",
			media_type: "application/pdf",
			name: "quarterly-infrastructure-cost-breakdown-for-every-workspace-template.pdf",
			file_id: "long-name-file",
		},
		{
			type: "file",
			media_type: "application/octet-stream",
			file_id: "unnamed-file",
		},
		{
			type: "file",
			media_type: "text/markdown",
			name: "PLAN.md",
			file_id: "plan-file",
		},
		{
			type: "file",
			media_type: "text/csv",
			name: "latency-report.csv",
			file_id: "report-file",
		},
		{
			type: "file",
			media_type: "image/png",
			name: "dashboard-screenshot.png",
			data: mockDashboardScreenshotPng,
		},
	],
};

const mockUploadMessage: TypesGen.ChatMessage = {
	...MockChatMessage,
	id: 2,
	content: [
		{ type: "text", text: "Here is the failing log." },
		{
			type: "file",
			media_type: "text/plain",
			name: "uploaded-ci.log",
			file_id: "upload-file",
		},
	],
};

const mockBrokenImageMessage: TypesGen.ChatMessage = {
	...MockChatMessage,
	role: "assistant",
	content: [
		{
			type: "file",
			media_type: "image/png",
			name: "broken-screenshot.png",
			// "not a png", base64-encoded.
			data: "bm90IGEgcG5n",
		},
	],
};

const meta: Meta<typeof ChatAttachments> = {
	title: "pages/AgentsPage/ChatAttachments",
	component: ChatAttachments,
	decorators: [
		(Story) => (
			<div className="w-90 p-4">
				<Story />
			</div>
		),
	],
	args: {
		attachments: getMessageAttachments([
			mockAttachmentMessage,
			mockUploadMessage,
		]),
		evictedFileIds: new Set(),
		isLoadingEarlierAttachments: false,
		earlierAttachmentsError: null,
		isRetrying: false,
		onRetry: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ChatAttachments>;

export const WithAttachments: Story = {};

export const Collapsed: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Attachments (6)" }),
		);
	},
};

export const HeaderFocused: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getByRole("button", { name: "Attachments (6)" })
			.focus();
	},
};

export const AttachmentFocused: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getByRole("button", { name: "PLAN.md View file" })
			.focus();
	},
};

export const DownloadLinkFocused: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getByRole("link", { name: "Download PLAN.md" })
			.focus();
	},
};

export const DownloadCardFocused: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getByRole("link", { name: "Download Attached file" })
			.focus();
	},
};

const viewPlan = async ({ canvasElement }: { canvasElement: HTMLElement }) => {
	await userEvent.click(
		within(canvasElement).getByRole("button", { name: "PLAN.md View file" }),
	);
};

export const TextFileLoading: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatFileText").mockReturnValue(
			new Promise(() => {}),
		);
	},
	play: viewPlan,
};

export const TextFileLoadingPreviewClosed: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatFileText").mockReturnValue(
			new Promise(() => {}),
		);
	},
	play: async (context) => {
		await viewPlan(context);
		await userEvent.keyboard("{Escape}");
	},
};

export const TextFileFailed: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatFileText").mockRejectedValue(
			mockApiError({ message: "Internal error reading the file." }),
		);
	},
	play: viewPlan,
};

export const TextFileExpired: Story = {
	beforeEach: () => {
		const notFound = mockApiError({ message: "File not found." });
		spyOn(API.experimental, "getChatFileText").mockRejectedValue({
			...notFound,
			response: { ...notFound.response, status: 404 },
		});
	},
	play: viewPlan,
};

export const ImageThumbnailFailed: Story = {
	args: { attachments: getMessageAttachments([mockBrokenImageMessage]) },
};

export const AgentAttachmentsOnly: Story = {
	args: { attachments: getMessageAttachments([mockAttachmentMessage]) },
};

export const UploadsOnly: Story = {
	args: { attachments: getMessageAttachments([mockUploadMessage]) },
};

export const AllAttachmentsExpired: Story = {
	args: {
		attachments: getMessageAttachments([mockUploadMessage]),
		evictedFileIds: new Set(["upload-file"]),
	},
};

export const Empty: Story = {
	args: { attachments: [] },
};

export const LoadingAttachments: Story = {
	args: { attachments: undefined },
};

export const LoadingEarlierAttachments: Story = {
	args: { isLoadingEarlierAttachments: true },
};

const mockEarlierAttachmentsError = mockApiError({
	message: "Failed to get chat attachments.",
});

export const EarlierAttachmentsFailed: Story = {
	args: { earlierAttachmentsError: mockEarlierAttachmentsError },
};

export const EarlierAttachmentsRetrying: Story = {
	args: {
		earlierAttachmentsError: mockEarlierAttachmentsError,
		isRetrying: true,
	},
};

export const EmptyWithEarlierAttachmentsFailed: Story = {
	args: {
		attachments: [],
		earlierAttachmentsError: mockEarlierAttachmentsError,
	},
};

export const ExpiredAttachment: Story = {
	args: { evictedFileIds: new Set(["report-file"]) },
};

export const ExpiredAttachmentsShown: Story = {
	args: {
		evictedFileIds: new Set(["report-file", "unnamed-file", "upload-file"]),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "3 expired attachments",
			}),
		);
	},
};
