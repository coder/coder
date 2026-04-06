import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import { PasswordField } from "./NewPasswordField";

const meta: Meta<typeof PasswordField> = {
	title: "components/NewPasswordField",
	component: PasswordField,
	args: {
		label: "Password",
		id: "password",
	},
	render: function StatefulPasswordField(args) {
		const [value, setValue] = useState("");
		return (
			<PasswordField
				{...args}
				value={value}
				onChange={(e) => setValue(e.currentTarget.value)}
			/>
		);
	},
};

export default meta;
type Story = StoryObj<typeof PasswordField>;

export const Idle: Story = {};

export const WithHelperText: Story = {
	args: {
		helperText: "Must be at least 8 characters.",
	},
};

const securePassword = "s3curePa$$w0rd";

export const Valid: Story = {
	play: async ({ canvasElement }) => {
		const validatePasswordSpy = spyOn(
			API,
			"validateUserPassword",
		).mockResolvedValueOnce({ valid: true, details: "" });
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const input = canvas.getByLabelText("Password");
		await user.type(input, securePassword);
		await waitFor(() =>
			expect(validatePasswordSpy).toHaveBeenCalledWith(securePassword),
		);
		expect(validatePasswordSpy).toHaveBeenCalledTimes(1);
	},
};

export const Invalid: Story = {
	play: async ({ canvasElement }) => {
		const validatePasswordSpy = spyOn(
			API,
			"validateUserPassword",
		).mockResolvedValueOnce({
			valid: false,
			details: "Password is too short.",
		});
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const input = canvas.getByLabelText("Password");
		await user.type(input, securePassword);
		await waitFor(() =>
			expect(validatePasswordSpy).toHaveBeenCalledWith(securePassword),
		);
		expect(validatePasswordSpy).toHaveBeenCalledTimes(1);
		await waitFor(() =>
			expect(canvas.getByText("Password is too short.")).toBeVisible(),
		);
	},
};
