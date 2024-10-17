import type { Meta, StoryObj } from "@storybook/react";
import { expect, spyOn, userEvent, within } from "@storybook/test";
import { API } from "api/api";
import { mockApiError } from "testHelpers/entities";
import { withGlobalSnackbar } from "testHelpers/storybook";
import ChangePasswordPage from "./ChangePasswordPage";

const meta: Meta<typeof ChangePasswordPage> = {
	title: "pages/ResetPasswordPage/ChangePasswordPage",
	component: ChangePasswordPage,
	args: { redirect: false },
	decorators: [withGlobalSnackbar],
};

export default meta;
type Story = StoryObj<typeof ChangePasswordPage>;

export const Default: Story = {};

export const Success: Story = {
	play: async ({ canvasElement }) => {
		spyOn(API, "changePasswordWithOTP").mockResolvedValueOnce();
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const newPasswordInput = await canvas.findByLabelText("Password *");
		await user.type(newPasswordInput, "password");
		const confirmPasswordInput =
			await canvas.findByLabelText("Confirm password *");
		await user.type(confirmPasswordInput, "password");
		await user.click(canvas.getByRole("button", { name: /reset password/i }));
		await canvas.findByText("Password reset successfully");
	},
};

export const WrongConfirmationPassword: Story = {
	play: async ({ canvasElement }) => {
		spyOn(API, "changePasswordWithOTP").mockRejectedValueOnce(
			mockApiError({
				message: "New password should be different from the old password",
			}),
		);
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const newPasswordInput = await canvas.findByLabelText("Password *");
		await user.type(newPasswordInput, "password");
		const confirmPasswordInput =
			await canvas.findByLabelText("Confirm password *");
		await user.type(confirmPasswordInput, "different-password");
		await user.click(canvas.getByRole("button", { name: /reset password/i }));
		await canvas.findByText("Passwords must match");
	},
};

export const ServerError: Story = {
	play: async ({ canvasElement }) => {
		const serverError =
			"New password should be different from the old password";
		spyOn(API, "changePasswordWithOTP").mockRejectedValueOnce(
			mockApiError({
				message: serverError,
			}),
		);
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const newPasswordInput = await canvas.findByLabelText("Password *");
		await user.type(newPasswordInput, "password");
		const confirmPasswordInput =
			await canvas.findByLabelText("Confirm password *");
		await user.type(confirmPasswordInput, "password");
		await user.click(canvas.getByRole("button", { name: /reset password/i }));
		await canvas.findByText(serverError);
	},
};
