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

export const CantConvertToDays: Story = {
	args: {
		valueMs: hoursToMs(2),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const unitDropdown = canvas.getByLabelText("Time unit");
		await userEvent.click(unitDropdown);
		const daysOption = within(document.body).getByText("Days");
		await expect(daysOption).toHaveAttribute("aria-disabled", "true");
	},
};

export const ConvertHoursToDays: Story = {
	args: {
		valueMs: hoursToMs(26), // 26 hours should convert to 2 days when switching units
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Initially should show 26 hours
		const input = canvas.getByLabelText("Duration");
		await expect(input).toHaveValue("26");

		// Switch to days by clicking the dropdown
		const unitDropdown = canvas.getByLabelText("Time unit");
		await userEvent.click(unitDropdown);

		// Find and click the Days option
		const daysOption = within(document.body).getByText("Days");
		await userEvent.click(daysOption);

		// After switching to days, the input should show 2 (rounded up from 26/24 = 1.08...)
		// and the component should have internally converted to 2 days worth of milliseconds
		await expect(input).toHaveValue("2");
	},
};

export const ConvertDaysToHours: Story = {
	args: {
		valueMs: daysToMs(2), // 2 days should convert to 48 hours when switching units
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Initially should show 2 days
		const input = canvas.getByLabelText("Duration");
		await expect(input).toHaveValue("2");

		// Switch to hours
		const unitDropdown = canvas.getByLabelText("Time unit");
		await userEvent.click(unitDropdown);

		const hoursOption = within(document.body).getByText("Hours");
		await userEvent.click(hoursOption);

		// After switching to hours, should show 48 hours (2 * 24)
		await expect(input).toHaveValue("48");
	},
};

export const BugFix26HoursToDays: Story = {
	args: {
		valueMs: hoursToMs(26), // Test the specific bug case
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Start with 26 hours displayed
		const input = canvas.getByLabelText("Duration");
		await expect(input).toHaveValue("26");

		// Switch to days - this is where the bug was happening
		const unitDropdown = canvas.getByLabelText("Time unit");
		await userEvent.click(unitDropdown);
		await userEvent.click(within(document.body).getByText("Days"));

		// The key fix: should show 2 days (26 hours = 1.083... days, rounded up = 2 days)
		// The component should correctly calculate this based on the original millisecond value
		await expect(input).toHaveValue("2");
	},
};

function hoursToMs(hours: number): number {
	return hours * 60 * 60 * 1000;
}

function daysToMs(days: number): number {
	return days * 24 * 60 * 60 * 1000;
}
