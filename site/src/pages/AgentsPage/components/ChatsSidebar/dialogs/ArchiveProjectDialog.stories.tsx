import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, spyOn } from "storybook/test";
import { API } from "#/api/api";
import { chatProjectArchivePreviewKey } from "#/api/queries/chatProjects";
import { chatRetentionDaysKey } from "#/api/queries/chats";
import {
	MockChatProject,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import { ArchiveProjectDialog } from "./ArchiveProjectDialog";

const meta = {
	title: "pages/AgentsPage/ArchiveProjectDialog",
	component: ArchiveProjectDialog,
	decorators: [withAuthProvider],
	args: {
		project: MockChatProject,
		open: true,
		onClose: fn(),
	},
	parameters: {
		user: MockUserOwner,
		queries: [
			{
				key: chatProjectArchivePreviewKey(MockChatProject.id),
				data: { chat_count: 5, active_chat_count: 0 },
			},
			{ key: chatRetentionDaysKey, data: { retention_days: 30 } },
		],
	},
} satisfies Meta<typeof ArchiveProjectDialog>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

/** Archived chats are kept forever, and two chats must be stopped. */
export const ActiveChats: Story = {
	parameters: {
		queries: [
			{
				key: chatProjectArchivePreviewKey(MockChatProject.id),
				data: { chat_count: 5, active_chat_count: 2 },
			},
			{ key: chatRetentionDaysKey, data: { retention_days: 0 } },
		],
	},
};

/** One running chat gets the singular warning. */
export const OneActiveChat: Story = {
	parameters: {
		queries: [
			{
				key: chatProjectArchivePreviewKey(MockChatProject.id),
				data: { chat_count: 5, active_chat_count: 1 },
			},
			{ key: chatRetentionDaysKey, data: { retention_days: 30 } },
		],
	},
};

/** A project without chats skips the count and the retention sentence. */
export const NoChats: Story = {
	parameters: {
		queries: [
			{
				key: chatProjectArchivePreviewKey(MockChatProject.id),
				data: { chat_count: 0, active_chat_count: 0 },
			},
			{ key: chatRetentionDaysKey, data: { retention_days: 30 } },
		],
	},
};

export const Loading: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		const pending = new Promise<never>(() => {});
		spyOn(API.experimental, "getChatProjectArchivePreview").mockReturnValue(
			pending,
		);
		spyOn(API.experimental, "getChatRetentionDays").mockReturnValue(pending);
	},
};

/** The chat count loaded, but the retention period did not. */
export const RetentionError: Story = {
	parameters: {
		queries: [
			{
				key: chatProjectArchivePreviewKey(MockChatProject.id),
				data: { chat_count: 5, active_chat_count: 0 },
			},
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatRetentionDays").mockRejectedValue(
			mockApiError({ message: "Failed to load the retention period." }),
		);
	},
};

export const PreviewError: Story = {
	parameters: {
		queries: [{ key: chatRetentionDaysKey, data: { retention_days: 30 } }],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjectArchivePreview").mockRejectedValue(
			mockApiError({ message: "Failed to count the project's chats." }),
		);
	},
};
