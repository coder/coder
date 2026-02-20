import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { DateRange } from "react-day-picker";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { Calendar } from "./Calendar";

// Fixed dates so snapshots are stable across runs.
const MONTH = new Date(2025, 5); // June 2025
const PREV_MONTH = new Date(2025, 4); // May 2025

const meta: Meta<typeof Calendar> = {
	title: "components/Calendar",
	component: Calendar,
	args: {
		defaultMonth: MONTH,
	},
};

export default meta;
type Story = StoryObj<typeof Calendar>;

export const Default: Story = {};

export const WithDropdowns: Story = {
	args: {
		captionLayout: "dropdown",
	},
};

export const TwoMonths: Story = {
	args: {
		numberOfMonths: 2,
		defaultMonth: PREV_MONTH,
	},
};

export const WithWeekNumbers: Story = {
	args: {
		showWeekNumber: true,
	},
};

export const SelectSingleDate: Story = {
	render: (args) => {
		const [selected, setSelected] = useState<Date | undefined>();
		return (
			<Calendar
				{...args}
				mode="single"
				selected={selected}
				onSelect={setSelected}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const day15 = canvas.getByText("15");
		await userEvent.click(day15);
		await waitFor(() =>
			expect(day15.closest("button")).toHaveAttribute(
				"data-selected-single",
				"true",
			),
		);
	},
};

export const ReselectSingleDate: Story = {
	render: (args) => {
		const [selected, setSelected] = useState<Date | undefined>();
		return (
			<Calendar
				{...args}
				mode="single"
				selected={selected}
				onSelect={setSelected}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Select the 10th.
		const day10 = canvas.getByText("10");
		await userEvent.click(day10);
		await waitFor(() =>
			expect(day10.closest("button")).toHaveAttribute(
				"data-selected-single",
				"true",
			),
		);

		// Select the 20th instead -- 10th should deselect.
		const day20 = canvas.getByText("20");
		await userEvent.click(day20);
		await waitFor(() => {
			expect(day20.closest("button")).toHaveAttribute(
				"data-selected-single",
				"true",
			);
			expect(day10.closest("button")).toHaveAttribute(
				"data-selected-single",
				"false",
			);
		});
	},
};

export const SelectRange: Story = {
	render: (args) => {
		const [selected, setSelected] = useState<DateRange | undefined>();
		return (
			<Calendar
				{...args}
				mode="range"
				selected={selected}
				onSelect={setSelected}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Click start of range.
		await userEvent.click(canvas.getByText("8"));
		await waitFor(() =>
			expect(canvas.getByText("8").closest("button")).toHaveAttribute(
				"data-range-start",
				"true",
			),
		);

		// Click end of range.
		await userEvent.click(canvas.getByText("14"));
		await waitFor(() => {
			expect(canvas.getByText("8").closest("button")).toHaveAttribute(
				"data-range-start",
				"true",
			);
			expect(canvas.getByText("14").closest("button")).toHaveAttribute(
				"data-range-end",
				"true",
			);
			expect(canvas.getByText("11").closest("button")).toHaveAttribute(
				"data-range-middle",
				"true",
			);
		});
	},
};

export const SelectRangeTwoMonths: Story = {
	args: {
		defaultMonth: PREV_MONTH,
	},
	render: (args) => {
		const [selected, setSelected] = useState<DateRange | undefined>();
		return (
			<Calendar
				{...args}
				mode="range"
				selected={selected}
				onSelect={setSelected}
				numberOfMonths={2}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const buttons = canvas.getAllByText("5");

		// Click the 5th of the first month.
		await userEvent.click(buttons[0]);
		await waitFor(() =>
			expect(buttons[0].closest("button")).toHaveAttribute(
				"data-range-start",
				"true",
			),
		);

		// Click the 5th of the second month.
		await userEvent.click(buttons[1]);
		await waitFor(() => {
			expect(buttons[0].closest("button")).toHaveAttribute(
				"data-range-start",
				"true",
			);
			expect(buttons[1].closest("button")).toHaveAttribute(
				"data-range-end",
				"true",
			);
		});
	},
};

export const PreselectedRange: Story = {
	render: (args) => {
		const [selected, setSelected] = useState<DateRange | undefined>({
			from: new Date(2025, 5, 5),
			to: new Date(2025, 5, 12),
		});
		return (
			<Calendar
				{...args}
				mode="range"
				selected={selected}
				onSelect={setSelected}
			/>
		);
	},
};

export const DisabledFutureDates: Story = {
	render: (args) => {
		const [selected, setSelected] = useState<Date | undefined>();
		return (
			<Calendar
				{...args}
				mode="single"
				selected={selected}
				onSelect={setSelected}
				disabled={{ after: new Date(2025, 5, 15) }}
			/>
		);
	},
};
