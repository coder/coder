import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { DateRangePicker, type DateRangeValue } from "./DateRangePicker";

const fixedNow = dayjs("2025-03-15T12:00:00Z");

const defaultValue: DateRangeValue = {
	startDate: fixedNow.subtract(30, "day").toDate(),
	endDate: fixedNow.toDate(),
};

const meta: Meta<typeof DateRangePicker> = {
	title: "components/DateRangePicker",
	component: DateRangePicker,
	args: {
		now: fixedNow.toDate(),
	},
};

export default meta;
type Story = StoryObj<typeof DateRangePicker>;

export const Closed: Story = {
	args: {
		value: defaultValue,
		onChange: () => {},
	},
};

export const Open: Story = {
	args: {
		value: defaultValue,
		onChange: () => {},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button");
		await userEvent.click(trigger);

		await waitFor(() => {
			expect(screen.getByText("Last 7 days")).toBeInTheDocument();
		});

		// All preset labels should be visible.
		expect(screen.getByText("Today")).toBeInTheDocument();
		expect(screen.getByText("Yesterday")).toBeInTheDocument();
		expect(screen.getByText("Last 14 days")).toBeInTheDocument();
		expect(screen.getByText("Last 30 days")).toBeInTheDocument();

		// The selected range should be displayed above the calendar.
		// The start date appears in both the trigger and the range
		// display, so expect two instances.
		const startDateLabel = dayjs(defaultValue.startDate).format("MMM D, YYYY");
		expect(screen.getAllByText(startDateLabel)).toHaveLength(2);

		// Cancel and Apply buttons should be visible.
		expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Apply" })).toBeInTheDocument();
	},
};

export const SelectPreset: Story = {
	render: function SelectPresetStory() {
		const [value, setValue] = useState<DateRangeValue>(defaultValue);
		return (
			<DateRangePicker
				value={value}
				onChange={setValue}
				now={fixedNow.toDate()}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		const trigger = canvas.getByRole("button");
		await userEvent.click(trigger);

		const preset = await body.findByText("Last 7 days");
		await userEvent.click(preset);

		// Popover should close after selecting a preset.
		await waitFor(() => {
			expect(screen.queryByText("Last 7 days")).toBeNull();
		});

		// The trigger button text should have changed to reflect a
		// narrower range than the original 30-day default.
		const updatedTrigger = canvas.getByRole("button");
		expect(updatedTrigger.textContent).not.toContain(
			dayjs(defaultValue.startDate).format("MMM D, YYYY"),
		);
	},
};

export const SelectCalendarRange: Story = {
	render: function SelectCalendarRangeStory() {
		const [value, setValue] = useState<DateRangeValue>({
			startDate: new Date("2025-03-01"),
			endDate: new Date("2025-03-15"),
		});
		return (
			<DateRangePicker
				value={value}
				onChange={setValue}
				now={fixedNow.toDate()}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(canvas.getByRole("button"));

		// Wait for the calendar to render.
		await waitFor(() => {
			expect(screen.getByText("Today")).toBeInTheDocument();
		});

		// The calendar should render day cells.
		const dayButtons = body.getAllByRole("gridcell");
		expect(dayButtons.length).toBeGreaterThan(0);

		// Apply button should be disabled until the range changes.
		const applyButton = screen.getByRole("button", { name: "Apply" });
		expect(applyButton).toBeDisabled();
	},
};

export const CancelClosesWithoutApplying: Story = {
	render: function CancelStory() {
		const [value, setValue] = useState<DateRangeValue>(defaultValue);
		return (
			<DateRangePicker
				value={value}
				onChange={setValue}
				now={fixedNow.toDate()}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const trigger = canvas.getByRole("button");
		const originalText = trigger.textContent;
		await userEvent.click(trigger);

		// Click Cancel.
		const cancelButton = await screen.findByRole("button", {
			name: "Cancel",
		});
		await userEvent.click(cancelButton);

		// Popover should close.
		await waitFor(() => {
			expect(screen.queryByText("Cancel")).toBeNull();
		});

		// The trigger text should remain unchanged.
		expect(canvas.getByRole("button").textContent).toBe(originalText);
	},
};

export const CustomPresets: Story = {
	args: {
		value: defaultValue,
		onChange: () => {},
		presets: [
			{
				label: "This week",
				range: () => ({
					from: dayjs().startOf("week").toDate(),
					to: new Date(),
				}),
			},
			{
				label: "This month",
				range: () => ({
					from: dayjs().startOf("month").toDate(),
					to: new Date(),
				}),
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));

		await waitFor(() => {
			expect(screen.getByText("This week")).toBeInTheDocument();
			expect(screen.getByText("This month")).toBeInTheDocument();
		});

		// Default presets should not be present.
		expect(screen.queryByText("Last 7 days")).toBeNull();
	},
};
