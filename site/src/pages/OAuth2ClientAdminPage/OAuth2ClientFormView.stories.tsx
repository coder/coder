import type { Meta, StoryObj } from "@storybook/react-vite";
import { OAuth2ClientFormView } from "./OAuth2ClientFormView";

const meta: Meta<typeof OAuth2ClientFormView> = {
	title: "pages/OAuth2ClientAdmin/Add",
	component: OAuth2ClientFormView,
	args: {
		onSubmit: () => {},
		onCancel: () => {},
	},
};

export default meta;
type Story = StoryObj<typeof OAuth2ClientFormView>;

/** Confidential is the default: it's the type that behaves the way admins expect. */
export const Confidential: Story = {};

/** Choosing public replaces the secret promise with the PKCE explanation, in place. */
export const Public: Story = {
	args: {
		initialType: "public",
	},
};

/** Submit stays enabled; validation fires on submit and attaches to the field. */
export const ValidationOnSubmit: Story = {
	play: async ({ canvas, userEvent }) => {
		await userEvent.click(
			await canvas.findByRole("button", { name: "Add application" }),
		);
	},
};

export const Submitting: Story = {
	args: {
		isSubmitting: true,
	},
};
