import type { Meta, StoryObj } from "@storybook/react-vite";
import { fireEvent, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { chatPreviews } from "#/api/queries/chats";
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

const meta = {
	title: "pages/AgentsPage/components/RightPanel/PreviewPanel",
	component: PreviewPanel,
	args: {
		chat: MockChat,
		workspace: MockWorkspace,
		host: "*.apps.example.com",
	},
	parameters: {
		layout: "centered",
		queries: [
			{
				key: chatPreviews(MockChat.id, MockWorkspace.id).queryKey,
				data: [MockChatPreview, MockChatFilePreview],
			},
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The sign-in frame's failed load can finish before this runs.
		const signIn = canvas.queryByTitle("Signing in to the preview");
		if (signIn) {
			fireEvent.load(signIn);
		}
		const frame = await canvas.findByTitle<HTMLIFrameElement>(
			MockChatPreview.title,
		);
		frame.srcdoc =
			'<!doctype html><html lang="en"><body style="margin:0;padding:24px;font:16px sans-serif;color:#1f2937;background:#fff"><h1>Preview app</h1></body></html>';
	},
};

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
				key: chatPreviews(MockChat.id, MockWorkspace.id).queryKey,
				data: [],
			},
		],
	},
};

export const Loading: Story = {
	parameters: {
		queries: [],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatPreviews").mockReturnValue(
			new Promise(() => {}),
		);
	},
};

export const PreviewsError: Story = {
	parameters: {
		queries: [],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatPreviews").mockRejectedValue(
			mockApiError({ message: "Couldn't load previews." }),
		);
	},
};
