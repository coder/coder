import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { chatProjectArchivePreviewKey } from "#/api/queries/chatProjects";
import { chatRetentionDaysKey } from "#/api/queries/chats";
import {
	MockChatProject,
	MockStoppedWorkspace,
	MockUserOwner,
	MockWorkspace,
} from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import {
	type ProjectChatWorkspace,
	projectChatWorkspacesKey,
} from "../../../utils/projectArchiveWorkspaces";
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
			{ key: chatRetentionDaysKey, data: { retention_days: 30 } },
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
			{ key: chatRetentionDaysKey, data: { retention_days: 0 } },
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
			{ key: chatRetentionDaysKey, data: { retention_days: 1 } },
			{
				key: projectChatWorkspacesKey(MockChatProject.id),
				data: mockWorkspaces,
			},
		],
	},
};

/** Deleting the workspaces asks for the project name. */
export const DeleteWorkspaces: Story = {
	parameters: Workspaces.parameters,
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("radio", { name: "Delete them" }),
		);
	},
};
