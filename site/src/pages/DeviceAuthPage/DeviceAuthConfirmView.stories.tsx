import type { Meta, StoryObj } from "@storybook/react-vite";
import { DeviceAuthConfirmView } from "./DeviceAuthConfirmView";

const meta: Meta<typeof DeviceAuthConfirmView> = {
	title: "pages/DeviceAuthPage/Confirm",
	component: DeviceAuthConfirmView,
	args: {
		userCode: "WDJB-MJHT",
		clientName: "Coder CLI",
		username: "admin",
		scopes: [
			{
				name: "workspace:read",
				description: "View your workspaces and their status",
			},
			{
				name: "workspace:ssh",
				description: "Connect to your workspaces over SSH",
			},
			{
				name: "template:read",
				description: "View templates you have access to",
			},
		],
		onApprove: () => {},
		onDeny: () => {},
	},
};

export default meta;
type Story = StoryObj<typeof DeviceAuthConfirmView>;

export const Default: Story = {};

export const LongClientName: Story = {
	args: {
		clientName: "Internal platform provisioning service (staging)",
	},
};

export const SingleScope: Story = {
	args: {
		clientName: "JetBrains Toolbox",
		scopes: [
			{
				name: "workspace:read",
				description: "View your workspaces and their status",
			},
		],
	},
};

export const Submitting: Story = {
	args: {
		isSubmitting: true,
	},
};
