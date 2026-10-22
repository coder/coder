import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { ChatAttachments } from "./ChatAttachments";
import { getMessageAttachments } from "./ChatConversation/messageHelpers";

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
			file_id: "screenshot-file",
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
		hasEarlierAttachmentsError: false,
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
			.getByRole("link", { name: "Download PLAN.md" })
			.focus();
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

export const EarlierAttachmentsFailed: Story = {
	args: { hasEarlierAttachmentsError: true },
};

export const EarlierAttachmentsRetrying: Story = {
	args: { hasEarlierAttachmentsError: true, isRetrying: true },
};

export const EmptyWithEarlierAttachmentsFailed: Story = {
	args: { attachments: [], hasEarlierAttachmentsError: true },
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
			within(canvasElement).getByRole("button", { name: "3 expired" }),
		);
	},
};
