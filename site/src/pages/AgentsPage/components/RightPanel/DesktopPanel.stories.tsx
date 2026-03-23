import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { DesktopPanelView, type DesktopPanelViewProps } from "./DesktopPanel";

const defaults: DesktopPanelViewProps = {
	status: "idle",
	reconnect: fn(),
	attach: fn(),
};

const meta: Meta<typeof DesktopPanelView> = {
	title: "pages/AgentsPage/DesktopPanel",
	component: DesktopPanelView,
	args: defaults,
	decorators: [
		(Story) => (
			<div style={{ height: 400, width: 480, border: "1px solid #333" }}>
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof DesktopPanelView>;

export const Idle: Story = {};

export const Connecting: Story = {
	args: { status: "connecting" },
};

export const Connected: Story = {
	args: { status: "connected" },
};

export const Disconnected: Story = {
	args: { status: "disconnected" },
};

export const ErrorState: Story = {
	args: { status: "error" },
};
