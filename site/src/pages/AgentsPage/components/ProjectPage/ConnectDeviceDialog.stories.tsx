import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ConnectDeviceDialogView } from "./ConnectedAccountsSection";

const meta: Meta<typeof ConnectDeviceDialogView> = {
	title: "pages/AgentsPage/ProjectPage/ConnectDeviceDialog",
	component: ConnectDeviceDialogView,
	args: {
		providerName: "GitLab",
		device: {
			device_code: "device-code",
			user_code: "ABCD-1234",
			verification_uri: "https://gitlab.example.com/device",
			expires_in: 900,
			interval: 5,
		},
		deviceExchangeError: undefined,
		onClose: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ConnectDeviceDialogView>;

export const WaitingForApproval: Story = {};

export const LoadingCode: Story = { args: { device: undefined } };

export const Expired: Story = {
	args: {
		deviceExchangeError: {
			message: "Failed to exchange.",
			detail: "expired_token",
		},
	},
};
