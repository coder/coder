import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import { PasswordField } from "./PasswordField";

const meta: Meta<typeof PasswordField> = {
	title: "components/PasswordField",
	component: PasswordField,
	args: {
		label: "Password",
		field: {
			id: "password",
			name: "password",
			error: false,
			onBlur: fn(),
			onChange: fn(),
		},
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
	},
};
