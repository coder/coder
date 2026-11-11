import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, spyOn, userEvent } from "storybook/test";
import { API } from "#/api/api";
import { chatProjectArchivePreviewKey } from "#/api/queries/chatProjects";
import { chatRetentionDays } from "#/api/queries/chats";
import {
	MockChatProject,
	MockStoppedWorkspace,
	MockUserOwner,
	MockWorkspace,
	mockApiError,
} from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import {
	type ProjectChatWorkspace,
	projectChatWorkspacesKey,
} from "../../../utils/agentWorkspaceUtils";
import { ArchiveProjectDialog } from "./ArchiveProjectDialog";

const mockWorkspaces: ProjectChatWorkspace[] = [
	{
		workspace: { ...MockWorkspace, id: "launch-docs", name: "launch-docs" },
		existedBeforeChat: false,
	},
	{
		workspace: { ...MockStoppedWorkspace, name: "my-dev-box" },
		existedBeforeChat: true,
	},
];

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
			{ key: chatRetentionDays().queryKey, data: { retention_days: 30 } },
			{ key: projectChatWorkspacesKey(MockChatProject.id), data: [] },
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
			{ key: chatRetentionDays().queryKey, data: { retention_days: 0 } },
			{ key: projectChatWorkspacesKey(MockChatProject.id), data: [] },
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
			{ key: chatRetentionDays().queryKey, data: { retention_days: 30 } },
			{ key: projectChatWorkspacesKey(MockChatProject.id), data: [] },
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
			{ key: chatRetentionDays().queryKey, data: { retention_days: 30 } },
			{ key: projectChatWorkspacesKey(MockChatProject.id), data: [] },
		],
	},
};

export const Workspaces: Story = {
	parameters: {
		queries: [
			{
				key: chatProjectArchivePreviewKey(MockChatProject.id),
				data: { chat_count: 1, active_chat_count: 0 },
			},
			{ key: chatRetentionDays().queryKey, data: { retention_days: 1 } },
			{
				key: projectChatWorkspacesKey(MockChatProject.id),
				data: mockWorkspaces,
			},
		],
	},
};

/** Deleting the workspaces asks for the project name. */
export const DeleteWorkspaces: Story = {
	parameters: {
		queries: [
			{
				key: chatProjectArchivePreviewKey(MockChatProject.id),
				data: { chat_count: 1, active_chat_count: 0 },
			},
			{ key: chatRetentionDays().queryKey, data: { retention_days: 1 } },
			{
				key: projectChatWorkspacesKey(MockChatProject.id),
				data: mockWorkspaces,
			},
		],
	},
	play: async () => {
		await userEvent.click(
			await screen.findByRole("radio", { name: "Delete them" }),
		);
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
		spyOn(API.experimental, "getChats").mockReturnValue(pending);
	},
};

/** Without the preview, the dialog can't tell whether chats are running. */
export const PreviewError: Story = {
	parameters: {
		queries: [
			{ key: chatRetentionDays().queryKey, data: { retention_days: 30 } },
			{ key: projectChatWorkspacesKey(MockChatProject.id), data: [] },
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjectArchivePreview").mockRejectedValue(
			mockApiError({ message: "Failed to count the project's chats." }),
		);
	},
};

export const WorkspacesError: Story = {
	parameters: {
		queries: [
			{
				key: chatProjectArchivePreviewKey(MockChatProject.id),
				data: { chat_count: 5, active_chat_count: 0 },
			},
			{ key: chatRetentionDays().queryKey, data: { retention_days: 30 } },
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChats").mockRejectedValue(
			mockApiError({ message: "Failed to list your chats." }),
		);
	},
};
