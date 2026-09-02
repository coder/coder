import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { DateTimeRangePicker } from "./DateTimeRangePicker";
import type { DateTimeRangeValue } from "./dateTimeRange";

// Matches the design mockup: mid-April 2026.
const fixedNow = new Date(2026, 3, 16, 10, 30, 0);

const presetValue: DateTimeRangeValue = {
	start: new Date(2026, 3, 16, 10, 15, 0),
	end: fixedNow,
	preset: "last_15m",
};

const customValue: DateTimeRangeValue = {
	start: new Date(2026, 3, 10, 0, 0, 0),
	end: new Date(2026, 3, 16, 0, 0, 0),
};

const meta: Meta<typeof DateTimeRangePicker> = {
	title: "components/DateTimeRangePicker",
	component: DateTimeRangePicker,
	args: {
		now: fixedNow,
		onChange: fn(),
	},
	// Every story is stateful so committing a selection updates the
	// trigger the same way it does in the app; onChange still reports
	// to the actions panel through the fn() spy.
	render: function StatefulPicker(args) {
		const [value, setValue] = useState(args.value);
		return (
			<DateTimeRangePicker
				{...args}
				value={value}
				onChange={(next) => {
					args.onChange(next);
					setValue(next);
				}}
			/>
		);
	},
};

export default meta;
type Story = StoryObj<typeof DateTimeRangePicker>;

export const Closed: Story = {
	args: {
		value: presetValue,
	},
};

export const ClosedWithCustomRange: Story = {
	args: {
		value: customValue,
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
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));

		await waitFor(() => {
			expect(
				screen.getByRole("radio", { name: "Custom range" }),
			).toBeInTheDocument();
		});
		const quickPicks = within(screen.getByRole("radiogroup"));

		expect(
			quickPicks.getByRole("radio", { name: "Last 15 min" }),
		).toBeInTheDocument();
		expect(
			quickPicks.getByRole("radio", { name: "Last hour" }),
		).toBeInTheDocument();
		expect(
			quickPicks.getByRole("radio", { name: "Last 24 hours" }),
		).toBeInTheDocument();
		expect(
			quickPicks.getByRole("radio", { name: "Today" }),
		).toBeInTheDocument();
		expect(
			quickPicks.getByRole("radio", { name: "This week" }),
		).toBeInTheDocument();
		expect(
			quickPicks.getByRole("radio", { name: "Last 15 min" }),
		).toBeChecked();

		// Calendar and time fields stay hidden until Custom range is chosen.
		expect(screen.queryByRole("grid")).toBeNull();
		expect(screen.queryByLabelText("From")).toBeNull();

		// Arrow keys move focus through the radiogroup.
		quickPicks.getByRole("radio", { name: "Last 15 min" }).focus();
		await userEvent.keyboard("{ArrowDown}");
		expect(quickPicks.getByRole("radio", { name: "Last hour" })).toHaveFocus();
		await userEvent.keyboard("{ArrowUp}{ArrowUp}");
		expect(
			quickPicks.getByRole("radio", { name: "Custom range" }),
		).toHaveFocus();
	},
};

export const SelectQuickPick: Story = {
	args: {
		value: presetValue,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));

		const preset = await screen.findByRole("radio", { name: "Last hour" });
		await userEvent.click(preset);

		// Selecting a quick pick commits immediately and closes the dropdown.
		await waitFor(() => {
			expect(screen.queryByRole("radio", { name: "Custom range" })).toBeNull();
		});
		expect(args.onChange).toHaveBeenCalledTimes(1);
		expect(
			canvas.getByRole("button", { name: /Last hour/ }),
		).toBeInTheDocument();
	},
};

export const CustomRangeExpanded: Story = {
	args: {
		value: presetValue,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));

		const customRadio = await screen.findByRole("radio", {
			name: "Custom range",
		});
		await userEvent.click(customRadio);

		// Calendar, time fields, and footer appear beside the quick picks.
		await waitFor(() => {
			expect(screen.getByRole("grid")).toBeInTheDocument();
		});
		expect(screen.getByLabelText("From")).toBeInTheDocument();
		expect(screen.getByLabelText("To")).toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();

		// Apply stays disabled until a full date range is selected.
		expect(screen.getByRole("button", { name: "Apply" })).toBeDisabled();

		// Future dates cannot be selected, and the calendar cannot page
		// past the current month.
		expect(
			screen.getByRole("button", { name: /April 17th, 2026/ }),
		).toBeDisabled();
		expect(screen.getByRole("button", { name: /next month/i })).toHaveAttribute(
			"aria-disabled",
			"true",
		);
	},
};

export const ApplyCustomRange: Story = {
	args: {
		value: presetValue,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));

		await userEvent.click(
			await screen.findByRole("radio", { name: "Custom range" }),
		);
		await waitFor(() => {
			expect(screen.getByRole("grid")).toBeInTheDocument();
		});

		const applyButton = screen.getByRole("button", { name: "Apply" });
		await userEvent.click(
			screen.getByRole("button", { name: /April 10th, 2026/ }),
		);
		await userEvent.click(
			screen.getByRole("button", { name: /April 16th, 2026/ }),
		);

		// Flip the To meridiem to exercise the select.
		await userEvent.click(
			screen.getByRole("combobox", { name: "To AM or PM" }),
		);
		await userEvent.click(await screen.findByRole("option", { name: "AM" }));

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

export const SelectSingleDay: Story = {
	args: {
		value: presetValue,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));

		await userEvent.click(
			await screen.findByRole("radio", { name: "Custom range" }),
		);
		await waitFor(() => {
			expect(screen.getByRole("grid")).toBeInTheDocument();
		});

		// A single calendar click is a complete one-day range spanning the
		// default 12:00:00 AM to 11:59:59 PM.
		await userEvent.click(
			screen.getByRole("button", { name: /April 10th, 2026/ }),
		);
		const applyButton = screen.getByRole("button", { name: "Apply" });
		await waitFor(() => {
			expect(applyButton).toBeEnabled();
		});
		await userEvent.click(applyButton);

		await waitFor(() => {
			expect(screen.queryByRole("grid")).toBeNull();
		});
		expect(
			canvas.getByRole("button", { name: /April 10, 12:00 AM - 11:59 PM/ }),
		).toBeInTheDocument();
	},
};

export const InvalidTimeDisablesApply: Story = {
	args: {
		value: customValue,
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

		// The message waits for blur, but Apply is blocked immediately.
		expect(screen.queryByRole("alert")).toBeNull();
		expect(screen.getByRole("button", { name: "Apply" })).toBeDisabled();
		await userEvent.tab();
		await waitFor(() => {
			expect(
				screen.getByText("Enter a valid time, e.g. 09:30:00"),
			).toBeInTheDocument();
		});
		expect(fromInput).toHaveAccessibleDescription(
			"Enter a valid time, e.g. 09:30:00",
		);

		// The message dismisses itself like a toast, while the invalid
		// styling and disabled Apply persist until the input is fixed.
		await waitFor(
			() => {
				expect(screen.queryByRole("alert")).toBeNull();
			},
			{ timeout: 7_000 },
		);
		expect(fromInput).toHaveAttribute("aria-invalid", "true");
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
			start: new Date(2026, 3, 10, 9, 0, 0),
			end: new Date(2026, 3, 10, 10, 0, 0),
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));

		await waitFor(() => {
			expect(screen.getByRole("grid")).toBeInTheDocument();
		});

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
	args: {
		value: presetValue,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button");
		const originalText = trigger.textContent;

		await userEvent.click(trigger);
		await userEvent.click(
			await screen.findByRole("radio", { name: "Custom range" }),
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

export const IntraDayTriggerLabel: Story = {
	args: {
		value: {
			start: new Date(2026, 3, 12, 9, 0, 0),
			end: new Date(2026, 3, 12, 11, 0, 0),
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: /April 12, 9:00 AM - 11:00 AM/ }),
		).toBeInTheDocument();
	},
};
