import { mockApiError } from "testHelpers/entities";
import { withGlobalSnackbar } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { spyOn, userEvent, within } from "storybook/test";
import RequestOTPPage from "./RequestOTPPage";

const meta: Meta<typeof RequestOTPPage> = {
	title: "pages/ResetPasswordPage/RequestOTPPage",
	component: RequestOTPPage,
	decorators: [withGlobalSnackbar],
};

export default meta;
type Story = StoryObj<typeof RequestOTPPage>;

export const Default: Story = {};

export const Success: Story = {
	play: async ({ canvasElement }) => {
		spyOn(API, "requestOneTimePassword").mockResolvedValueOnce();
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const emailInput = await canvas.findByLabelText(/email/i);
		await user.type(emailInput, "admin@coder.com");
		await user.click(canvas.getByRole("button", { name: /reset password/i }));
	},
};

export const ServerError: Story = {
	play: async ({ canvasElement }) => {
		spyOn(API, "requestOneTimePassword").mockRejectedValueOnce(
			mockApiError({
				message: "Error requesting password change",
			}),
		);
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const emailInput = await canvas.findByLabelText(/email/i);
		await user.type(emailInput, "admin@coder.com");
		await user.click(canvas.getByRole("button", { name: /reset password/i }));
	},
};
