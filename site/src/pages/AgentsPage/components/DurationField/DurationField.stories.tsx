import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { DurationField } from "./DurationField";

const meta: Meta<typeof DurationField> = {
	title: "pages/AgentsPage/DurationField",
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
		const unitTrigger = canvas.getByLabelText("Time unit");
		await userEvent.click(unitTrigger);
		const hoursOption = await within(document.body).findByText("Hours");
		await userEvent.click(hoursOption);
		await expect(input).toHaveValue("48");
	},
};

export const ConvertSmallHoursToDays: Story = {
	args: {
		valueMs: hoursToMs(2),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const input = canvas.getByLabelText("Duration");
		await expect(input).toHaveValue("2");

		const unitTrigger = canvas.getByLabelText("Time unit");
		await userEvent.click(unitTrigger);

		const daysOption = await within(document.body).findByText("Days");
		await userEvent.click(daysOption);

		await expect(input).toHaveValue("1");
	},
};

export const WithError: Story = {
	args: {
		valueMs: hoursToMs(1),
		error: true,
		helperText: "Duration must be greater than zero.",
	},
};

function hoursToMs(hours: number): number {
	return hours * 60 * 60 * 1000;
}

function daysToMs(days: number): number {
	return days * 24 * 60 * 60 * 1000;
}
