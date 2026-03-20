import type { Meta, StoryObj } from "@storybook/react-vite";
import { DesktopPanel } from "./DesktopPanel";
import { mockAttach, mockDesktopConnection } from "./desktopStoryUtils";

const meta: Meta<typeof DesktopPanel> = {
	title: "pages/AgentsPage/DesktopPanel",
	component: DesktopPanel,
	args: {
		isExpanded: false,
		chatId: "test-chat-id",
	},
	decorators: [
		(Story) => (
			<div style={{ height: 400, width: 480, border: "1px solid #333" }}>
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof DesktopPanel>;

export const Connected: Story = {
	args: {
		connectionOverride: mockDesktopConnection({
			status: "connected",
			hasConnected: true,
			attach: mockAttach(),
		}),
	},
};

export const Connecting: Story = {
	args: {
		connectionOverride: mockDesktopConnection({ status: "connecting" }),
	},
};

export const ErrorState: Story = {
	args: {
		connectionOverride: mockDesktopConnection({ status: "error" }),
	},
};

export const Disconnected: Story = {
	args: {
		connectionOverride: mockDesktopConnection({
			status: "disconnected",
			hasConnected: true,
		}),
	},
};
