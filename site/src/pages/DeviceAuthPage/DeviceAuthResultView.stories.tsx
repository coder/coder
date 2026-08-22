import type { Meta, StoryObj } from "@storybook/react-vite";
import { DeviceAuthResultView } from "./DeviceAuthResultView";

const meta: Meta<typeof DeviceAuthResultView> = {
	title: "pages/DeviceAuthPage/Result",
	component: DeviceAuthResultView,
	args: {
		clientName: "Coder CLI",
	},
};

export default meta;
type Story = StoryObj<typeof DeviceAuthResultView>;

export const Approved: Story = {
	args: {
		result: "approved",
	},
};

export const Denied: Story = {
	args: {
		result: "denied",
	},
};

export const Expired: Story = {
	args: {
		result: "expired",
		onStartOver: () => {},
	},
};
