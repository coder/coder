import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	expect,
	fireEvent,
	fn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { formatDateTime } from "#/utils/time";
import { DateTimeRangeFilter } from "./DateTimeRangeFilter";
import { type TimeRange, toLocalInputValue } from "./timeRange";

const fixedNow = new Date("2026-08-13T15:00:00Z");

const defaultValue: TimeRange = {
	startedAfter: new Date("2026-08-12T15:00:00Z"),
	startedBefore: fixedNow,
};

// Local (zone-less) timestamps so the trigger label matches whatever
// timezone the test runner uses.
const explicitValue: TimeRange = {
	startedAfter: new Date("2026-08-01T09:30:00"),
	startedBefore: new Date("2026-08-02T17:45:00"),
};

const meta: Meta<typeof DateTimeRangeFilter> = {
	title: "pages/AIBridgePage/DateTimeRangeFilter",
	component: DateTimeRangeFilter,
	args: {
		now: fixedNow,
		value: defaultValue,
		isDefault: true,
		onChange: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof DateTimeRangeFilter>;

export const DefaultLabel: Story = {
	args: {
		onChange: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button", {
			name: "Filter by time range",
		});
		expect(trigger).toHaveTextContent("Last 24 hours");
		expect(args.onChange).not.toHaveBeenCalled();
	},
};

export const ExplicitRangeLabel: Story = {
	args: {
		isDefault: false,
		value: explicitValue,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button", {
			name: "Filter by time range",
		});
		expect(trigger.textContent).toContain(
			formatDateTime(explicitValue.startedAfter, "MMM D, HH:mm"),
		);
		expect(trigger.textContent).toContain(
			formatDateTime(explicitValue.startedBefore, "MMM D, HH:mm"),
		);
	},
};

export const OpenShowsInputs: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("button", { name: "Filter by time range" }),
		);

		// Inputs render in a portal and are populated with the committed
		// value in browser-local time.
		const startInput = await body.findByLabelText("Start of time range");
		const endInput = body.getByLabelText("End of time range");
		expect(startInput).toHaveValue(
			toLocalInputValue(defaultValue.startedAfter),
		);
		expect(endInput).toHaveValue(toLocalInputValue(defaultValue.startedBefore));

		// Apply stays disabled until the selection changes.
		expect(body.getByRole("button", { name: "Apply" })).toBeDisabled();
	},
};

export const ApplyCommitsSelection: Story = {
	args: {
		onChange: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("button", { name: "Filter by time range" }),
		);

		const startInput = await body.findByLabelText("Start of time range");
		await fireEvent.change(startInput, {
			target: { value: "2026-08-13T08:00" },
		});

		const applyButton = body.getByRole("button", { name: "Apply" });
		expect(applyButton).toBeEnabled();
		await userEvent.click(applyButton);

		// The popover closes and onChange receives the committed range. The
		// input string is parsed as browser-local time, matching how the
		// component interprets it.
		await waitFor(() => {
			expect(body.queryByRole("button", { name: "Apply" })).toBeNull();
		});
		expect(args.onChange).toHaveBeenCalledWith({
			startedAfter: new Date("2026-08-13T08:00"),
			startedBefore: defaultValue.startedBefore,
		});
	},
};

export const InvalidRangeDisablesApply: Story = {
	args: {
		onChange: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("button", { name: "Filter by time range" }),
		);

		// A start at or after the end is not a valid range.
		const startInput = await body.findByLabelText("Start of time range");
		await fireEvent.change(startInput, {
			target: { value: toLocalInputValue(defaultValue.startedBefore) },
		});
		expect(body.getByRole("button", { name: "Apply" })).toBeDisabled();

		// Empty values are ignored, so the committed value remains and Apply
		// stays disabled for an unchanged selection.
		await fireEvent.change(startInput, { target: { value: "" } });
		expect(body.getByRole("button", { name: "Apply" })).toBeDisabled();

		await userEvent.click(body.getByRole("button", { name: "Cancel" }));
		expect(args.onChange).not.toHaveBeenCalled();
	},
};

export const CancelClosesWithoutApplying: Story = {
	args: {
		onChange: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("button", { name: "Filter by time range" }),
		);

		const startInput = await body.findByLabelText("Start of time range");
		await fireEvent.change(startInput, {
			target: { value: "2026-08-13T08:00" },
		});
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		await waitFor(() => {
			expect(body.queryByRole("button", { name: "Apply" })).toBeNull();
		});
		expect(args.onChange).not.toHaveBeenCalled();
	},
};
