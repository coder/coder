import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { DesktopToolbar } from "./DesktopToolbar";

const meta = {
	title: "pages/AgentsPage/DesktopToolbar",
	component: DesktopToolbar,
} satisfies Meta<typeof DesktopToolbar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ViewOnly: Story = {
	args: {
		scaleMode: "fit",
		onScaleModeChange: fn(),
		isControlling: false,
		onTakeControl: fn(),
		onReleaseControl: fn(),
		onPopOut: fn(),
	},
};

export const Controlling: Story = {
	args: {
		...ViewOnly.args,
		isControlling: true,
	},
};

export const NativeZoom: Story = {
	args: {
		...ViewOnly.args,
		scaleMode: "native",
	},
};

export const PoppedOut: Story = {
	args: {
		...ViewOnly.args,
		isPoppedOut: true,
	},
};
