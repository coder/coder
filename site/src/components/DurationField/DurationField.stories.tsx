import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { DurationField } from "./DurationField";

const meta: Meta<typeof DurationField> = {
	title: "components/DurationField",
	component: DurationField,
	args: {
		label: "Duration",
	},
	render: function RenderComponent(args) {
		const [value, setValue] = useState<number>(args.valueMs);
		return (
			<DurationField
				{...args}
				valueMs={value}
				onChange={(value) => setValue(value)}
			/>
		);
	},
};

export default meta;
type Story = StoryObj<typeof DurationField>;

export const Hours: Story = {
	args: {
		valueMs: hoursToMs(16),
	},
};

export const Days: Story = {
	args: {
		valueMs: daysToMs(2),
	},
};

export const TypeOnlyNumbers: Story = {
	args: {
		valueMs: 0,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByLabelText("Duration");
		await userEvent.clear(input);
		await userEvent.type(input, "abcd_.?/48.0");
		await expect(input).toHaveValue("480");
	},
};

export const ChangeUnit: Story = {
	args: {
		valueMs: daysToMs(2),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByLabelText("Duration");
		const unitDropdown = canvas.getByLabelText("Time unit");
		await userEvent.click(unitDropdown);
		const hoursOption = within(document.body).getByText("Hours");
		await userEvent.click(hoursOption);
		await expect(input).toHaveValue("48");
	},
};

export const ConvertSmallHoursToDays: Story = {
	args: {
		valueMs: hoursToMs(2), // 2 hours should convert to 1 day when switching units (rounded up)
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Initially should show 2 hours
		const input = canvas.getByLabelText("Duration");
		await expect(input).toHaveValue("2");

		// Switch to days by clicking the dropdown
		const unitDropdown = canvas.getByLabelText("Time unit");
		await userEvent.click(unitDropdown);

		// Find and click the Days option - this should now work (no longer disabled)
		const daysOption = within(document.body).getByText("Days");
		await userEvent.click(daysOption);

		// After switching to days, should show 1 day (2 hours rounded up to nearest day)
		await expect(input).toHaveValue("1");
	},
};

function hoursToMs(hours: number): number {
	return hours * 60 * 60 * 1000;
}

function daysToMs(days: number): number {
	return days * 24 * 60 * 60 * 1000;
}
