import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { ChatAttachments } from "./ChatAttachments";
import { getAgentAttachments } from "./ChatConversation/messageHelpers";

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
		attachments: getAgentAttachments([mockAttachmentMessage]),
		evictedFileIds: new Set(),
		hasMoreMessages: false,
		isFetchingMoreMessages: false,
		hasFetchMoreError: false,
		onFetchMoreMessages: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ChatAttachments>;

export const WithAttachments: Story = {};

export const Collapsed: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Attachments (5)" }),
		);
	},
};

export const HeaderFocused: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getByRole("button", { name: "Attachments (5)" })
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

export const EmptyWithEarlierMessages: Story = {
	args: { attachments: [], hasMoreMessages: true },
};

export const WithEarlierMessages: Story = {
	args: { hasMoreMessages: true },
};

export const LoadingEarlierMessages: Story = {
	args: { hasMoreMessages: true, isFetchingMoreMessages: true },
};

export const LoadEarlierMessagesFailed: Story = {
	args: { hasMoreMessages: true, hasFetchMoreError: true },
};

export const LoadEarlierMessagesRetrying: Story = {
	args: {
		hasMoreMessages: true,
		hasFetchMoreError: true,
		isFetchingMoreMessages: true,
	},
};

export const ExpiredAttachment: Story = {
	args: { evictedFileIds: new Set(["report-file"]) },
};
