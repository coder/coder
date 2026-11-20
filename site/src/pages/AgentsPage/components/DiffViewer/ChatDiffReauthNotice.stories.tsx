import type { Meta, StoryObj } from "@storybook/react-vite";
import { MockChatDiffReauthRequired } from "#/testHelpers/chatEntities";
import { ChatDiffReauthNotice } from "./ChatDiffReauthNotice";

const meta: Meta<typeof ChatDiffReauthNotice> = {
	title: "pages/AgentsPage/ChatDiffReauthNotice",
	component: ChatDiffReauthNotice,
	args: {
		chatId: "test-chat",
		reauth: MockChatDiffReauthRequired,
	},
	decorators: [
		(Story) => (
			<div style={{ height: 300, width: 480 }}>
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ChatDiffReauthNotice>;

export const OwnerCanReconnect: Story = {};

export const ViewerNeedsOwner: Story = {
	args: {
		reauth: {
			...MockChatDiffReauthRequired,
			message:
				"GitHub rejected the chat owner's saved credentials. Ask the chat owner to reconnect GitHub.",
			can_reconnect: false,
		},
	},
};
