import type { Meta, StoryObj } from "@storybook/react-vite";
import { DeviceAuthEnterCodeView } from "./DeviceAuthEnterCodeView";

const meta: Meta<typeof DeviceAuthEnterCodeView> = {
	title: "pages/DeviceAuthPage/EnterCode",
	component: DeviceAuthEnterCodeView,
	args: {
		onSubmit: () => {},
	},
};

export default meta;
type Story = StoryObj<typeof DeviceAuthEnterCodeView>;

export const Empty: Story = {};

export const Prefilled: Story = {
	args: {
		initialUserCode: "WDJB-MJHT",
	},
};

export const Submitting: Story = {
	args: {
		initialUserCode: "WDJB-MJHT",
		isSubmitting: true,
	},
};

export const InvalidCode: Story = {
	args: {
		initialUserCode: "WDJB-MJHT",
		error: "invalid",
	},
};

export const ExpiredCode: Story = {
	args: {
		initialUserCode: "WDJB-MJHT",
		error: "expired",
	},
};
