import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn } from "storybook/test";
import { DesktopPanelView, type DesktopPanelViewProps } from "./DesktopPanel";

const defaults: DesktopPanelViewProps = {
	status: "idle",
	reconnect: fn(),
	attach: fn(),
	isControlling: false,
	onTakeControl: fn(),
	onReleaseControl: fn(),
};

const meta: Meta<typeof DesktopPanelView> = {
	title: "pages/AgentsPage/DesktopPanel",
	component: DesktopPanelView,
	args: defaults,
	decorators: [
		(Story) => (
			<div
				style={{
					height: 400,
					width: 480,
					border: "1px solid #333",
					background:
						"linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%)",
				}}
			>
				<Story />
			</div>
		),
	],
	render: function RenderComponent(args) {
		const [isControlling, setIsControlling] = useState(args.isControlling);
		return (
			<DesktopPanelView
				{...args}
				isControlling={isControlling}
				onTakeControl={() => setIsControlling(true)}
				onReleaseControl={() => setIsControlling(false)}
			/>
		);
	},
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

export const ConnectedControlling: Story = {
	args: { status: "connected", isControlling: true },
};

export const Disconnected: Story = {
	args: { status: "disconnected" },
};

export const ErrorState: Story = {
	args: { status: "error" },
};
