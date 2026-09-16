import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { DesktopPopoutPageView } from "./DesktopPopoutPage";

const meta = {
	title: "pages/AgentsPage/DesktopPopoutPage",
	component: DesktopPopoutPageView,
	parameters: {
		layout: "fullscreen",
	},
} satisfies Meta<typeof DesktopPopoutPageView>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Connecting: Story = {
	args: {
		status: "connecting",
		reconnect: fn(),
		attach: fn(),
		scaleMode: "fit",
		onScaleModeChange: fn(),
		isControlling: false,
		onTakeControl: fn(),
		onReleaseControl: fn(),
	},
};

export const Connected: Story = {
	args: {
		...Connecting.args,
		status: "connected",
	},
};

export const ErrorState: Story = {
	args: {
		...Connecting.args,
		status: "error",
	},
};

export const Disconnected: Story = {
	args: {
		...Connecting.args,
		status: "disconnected",
	},
};
