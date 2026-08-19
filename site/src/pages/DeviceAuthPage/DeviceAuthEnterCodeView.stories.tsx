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

export const NotRecognized: Story = {
	args: {
		initialUserCode: "WDJB-MJHT",
		error: "not-recognized",
	},
};

export const ExpiredCode: Story = {
	args: {
		initialUserCode: "WDJB-MJHT",
		error: "expired",
	},
};

/** Submitting an incomplete code: the button stays enabled and the field explains. */
export const IncompleteCode: Story = {
	args: {
		initialUserCode: "WDJB",
	},
	play: async ({ canvas, userEvent }) => {
		await userEvent.click(
			await canvas.findByRole("button", { name: "Continue" }),
		);
	},
};
