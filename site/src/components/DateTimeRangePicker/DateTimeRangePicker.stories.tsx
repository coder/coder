import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { DateTimeRangePicker } from "./DateTimeRangePicker";
import type { DateTimeRange } from "./dateTimeRange";

// Matches the design mockup: mid-April 2026.
const fixedNow = new Date(2026, 3, 16, 10, 30, 0);

const presetValue: DateTimeRange = { type: "preset", preset: "last_15m" };

const customValue: DateTimeRange = {
	type: "custom",
	start: new Date(2026, 3, 10, 0, 0, 0),
	end: new Date(2026, 3, 16, 0, 0, 0),
};

const meta: Meta<typeof DateTimeRangePicker> = {
	title: "components/DateTimeRangePicker",
	component: DateTimeRangePicker,
	args: {
		now: fixedNow,
	},
};

export default meta;
type Story = StoryObj<typeof DateTimeRangePicker>;

export const Closed: Story = {
	args: {
		value: presetValue,
		onChange: () => {},
	},
};

export const ClosedWithCustomRange: Story = {
	args: {
		value: customValue,
		onChange: () => {},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: /April 10-16/ }),
		).toBeInTheDocument();
	},
};

export const OpenShowsOnlyQuickPicks: Story = {
	args: {
		value: presetValue,
		onChange: () => {},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));

		await waitFor(() => {
			expect(
				screen.getByRole("button", { name: "Custom range" }),
			).toBeInTheDocument();
		});
		const popover = within(screen.getByRole("dialog"));

		expect(
			popover.getByRole("button", { name: "Last 15 min" }),
		).toBeInTheDocument();
		expect(
			popover.getByRole("button", { name: "Last hour" }),
		).toBeInTheDocument();
		expect(popover.getByRole("button", { name: "Today" })).toBeInTheDocument();
		expect(
			popover.getByRole("button", { name: "This week" }),
		).toBeInTheDocument();

		// The active preset is marked as selected.
		expect(
			popover.getByRole("button", { name: "Last 15 min" }),
		).toHaveAttribute("aria-pressed", "true");

		// Calendar and time fields stay hidden until Custom range is chosen.
		expect(screen.queryByRole("grid")).toBeNull();
		expect(screen.queryByLabelText("From")).toBeNull();
	},
};

export const SelectQuickPick: Story = {
	render: function SelectQuickPickStory() {
		const [value, setValue] = useState<DateTimeRange>(presetValue);
		return (
			<DateTimeRangePicker value={value} onChange={setValue} now={fixedNow} />
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));

		const preset = await screen.findByRole("button", { name: "Last hour" });
		await userEvent.click(preset);

		// Selecting a quick pick commits immediately and closes the dropdown.
		await waitFor(() => {
			expect(screen.queryByRole("button", { name: "Custom range" })).toBeNull();
		});
		expect(
			canvas.getByRole("button", { name: /Last hour/ }),
		).toBeInTheDocument();
	},
};

export const CustomRangeExpanded: Story = {
	args: {
		value: presetValue,
		onChange: () => {},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));

		const customButton = await screen.findByRole("button", {
			name: "Custom range",
		});
		await userEvent.click(customButton);

		// Calendar, time fields, and footer appear beside the quick picks.
		await waitFor(() => {
			expect(screen.getByRole("grid")).toBeInTheDocument();
		});
		expect(screen.getByLabelText("From")).toBeInTheDocument();
		expect(screen.getByLabelText("To")).toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();

		// Apply stays disabled until a date range is selected.
		expect(screen.getByRole("button", { name: "Apply" })).toBeDisabled();
	},
};

export const ApplyCustomRange: Story = {
	render: function ApplyCustomRangeStory() {
		const [value, setValue] = useState<DateTimeRange>(presetValue);
		return (
			<DateTimeRangePicker value={value} onChange={setValue} now={fixedNow} />
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));

		await userEvent.click(
			await screen.findByRole("button", { name: "Custom range" }),
		);
		await waitFor(() => {
			expect(screen.getByRole("grid")).toBeInTheDocument();
		});

		// Pick April 10 through April 16 on the calendar.
		await userEvent.click(
			screen.getByRole("button", { name: /April 10th, 2026/ }),
		);
		await userEvent.click(
			screen.getByRole("button", { name: /April 16th, 2026/ }),
		);

		const applyButton = screen.getByRole("button", { name: "Apply" });
		await waitFor(() => {
			expect(applyButton).toBeEnabled();
		});
		await userEvent.click(applyButton);

		// The dropdown closes and the trigger shows the compact range.
		await waitFor(() => {
			expect(screen.queryByRole("grid")).toBeNull();
		});
		expect(
			canvas.getByRole("button", { name: /April 10-16/ }),
		).toBeInTheDocument();
	},
};

export const InvalidTimeDisablesApply: Story = {
	args: {
		value: customValue,
		onChange: () => {},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));

		// Reopening with a committed custom value restores the expanded panel.
		await waitFor(() => {
			expect(screen.getByRole("grid")).toBeInTheDocument();
		});

		const fromInput = screen.getByLabelText("From");
		await userEvent.clear(fromInput);
		await userEvent.type(fromInput, "99:99");

		await waitFor(() => {
			expect(
				screen.getByText("Enter a valid time, e.g. 09:30:00"),
			).toBeInTheDocument();
		});
		expect(screen.getByRole("button", { name: "Apply" })).toBeDisabled();

		// Fixing the time re-enables Apply.
		await userEvent.clear(fromInput);
		await userEvent.type(fromInput, "09:15:00");
		await waitFor(() => {
			expect(screen.getByRole("button", { name: "Apply" })).toBeEnabled();
		});
	},
};

export const EndBeforeStartShowsError: Story = {
	args: {
		value: {
			type: "custom",
			start: new Date(2026, 3, 10, 0, 0, 0),
			end: new Date(2026, 3, 10, 0, 0, 0),
		},
		onChange: () => {},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));

		await waitFor(() => {
			expect(screen.getByRole("grid")).toBeInTheDocument();
		});

		// Same day with To earlier than From is rejected.
		const fromInput = screen.getByLabelText("From");
		await userEvent.clear(fromInput);
		await userEvent.type(fromInput, "11:00:00");

		await waitFor(() => {
			expect(screen.getByText("End must be after start")).toBeInTheDocument();
		});
		expect(screen.getByRole("button", { name: "Apply" })).toBeDisabled();
	},
};

export const CancelDiscardsDraft: Story = {
	render: function CancelDiscardsDraftStory() {
		const [value, setValue] = useState<DateTimeRange>(presetValue);
		return (
			<DateTimeRangePicker value={value} onChange={setValue} now={fixedNow} />
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button");
		const originalText = trigger.textContent;

		await userEvent.click(trigger);
		await userEvent.click(
			await screen.findByRole("button", { name: "Custom range" }),
		);
		await userEvent.click(
			screen.getByRole("button", { name: /April 10th, 2026/ }),
		);
		await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

		await waitFor(() => {
			expect(screen.queryByRole("grid")).toBeNull();
		});
		expect(canvas.getByRole("button").textContent).toBe(originalText);
	},
};
