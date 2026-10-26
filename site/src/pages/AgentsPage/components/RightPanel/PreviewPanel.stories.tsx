import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import { chatPreviewsKey } from "#/api/queries/chats";
import {
	MockChat,
	MockChatFilePreview,
	MockChatPreview,
} from "#/testHelpers/chatEntities";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceResource,
	mockApiError,
} from "#/testHelpers/entities";
import { PreviewPanel } from "./PreviewPanel";

const previewAccessKey = getAuthorizationKey({
	checks: {
		canOpenPreviews: {
			object: {
				resource_type: "workspace",
				resource_id: MockWorkspace.id,
				owner_id: MockWorkspace.owner_id,
			},
			action: "application_connect",
		},
	},
});

const meta = {
	title: "pages/AgentsPage/components/RightPanel/PreviewPanel",
	component: PreviewPanel,
	args: {
		chat: MockChat,
		workspace: MockWorkspace,
		host: "*.apps.example.com",
		isVisible: true,
	},
	parameters: {
		layout: "centered",
		queries: [
			{
				key: chatPreviewsKey(MockChat.id, MockWorkspace.id),
				data: [MockChatPreview, MockChatFilePreview],
			},
			{ key: previewAccessKey, data: { canOpenPreviews: true } },
		],
	},
	decorators: [
		(Story) => (
			<div className="h-105 w-120">
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof PreviewPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Preview: Story = {
	// The workspace URL does not resolve in Storybook.
	play: ({ canvasElement }) => {
		const frame = within(canvasElement).getByTitle<HTMLIFrameElement>(
			MockChatPreview.title,
		);
		frame.srcdoc =
			'<!doctype html><html lang="en"><body style="margin:0;padding:24px;font:16px sans-serif;color:#1f2937;background:#fff"><h1>Preview app</h1></body></html>';
	},
};

// The workspace is running, but the preview's agent has not connected yet.
export const AgentDisconnected: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				resources: [
					{
						...MockWorkspaceResource,
						agents: [{ ...MockWorkspaceAgent, status: "disconnected" }],
					},
				],
			},
		},
	},
};

export const DropdownOpen: Story = {
	play: async (context) => {
		await Preview.play?.(context);
		await userEvent.click(within(context.canvasElement).getByRole("combobox"));
	},
};

export const Empty: Story = {
	parameters: {
		queries: [
			{
				key: chatPreviewsKey(MockChat.id, MockWorkspace.id),
				data: [],
			},
			{ key: previewAccessKey, data: { canOpenPreviews: true } },
		],
	},
};

export const PreviewsError: Story = {
	parameters: {
		queries: [{ key: previewAccessKey, data: { canOpenPreviews: true } }],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatPreviews").mockRejectedValue(
			mockApiError({ message: "Couldn't load previews." }),
		);
	},
};

export const NoAccess: Story = {
	parameters: {
		queries: [
			{
				key: chatPreviewsKey(MockChat.id, MockWorkspace.id),
				data: [MockChatPreview, MockChatFilePreview],
			},
			{ key: previewAccessKey, data: { canOpenPreviews: false } },
		],
	},
};
