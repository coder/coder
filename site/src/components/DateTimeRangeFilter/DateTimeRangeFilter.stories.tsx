import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	expect,
	fireEvent,
	fn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import type { TimeRange } from "#/pages/AIBridgePage/ListSessionsPage/timeRange";
import { formatDateTime } from "#/utils/time";
import { DateTimeRangeFilter } from "./DateTimeRangeFilter";

const fixedNow = new Date(2026, 7, 13, 15, 0, 0);

const defaultValue: TimeRange = {
	startedAfter: new Date(2026, 7, 12, 15, 0, 0),
	startedBefore: fixedNow,
};

const singleDayValue: TimeRange = {
	startedAfter: new Date(2026, 3, 10, 7, 23, 0),
	startedBefore: new Date(2026, 3, 10, 9, 30, 0),
};

const meta: Meta<typeof DateTimeRangeFilter> = {
	title: "components/DateTimeRangeFilter",
	component: DateTimeRangeFilter,
	args: {
		now: fixedNow,
		value: defaultValue,
		defaultValue,
		onChange: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof DateTimeRangeFilter>;

export const DefaultLabel: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: "Filter by time range" }),
		).toHaveTextContent("Last 24 hours");
		expect(args.onChange).not.toHaveBeenCalled();
	},
};

export const SingleDayLabel: Story = {
	args: {
		value: singleDayValue,
	},
};

export const RangeEndingTodayLabel: Story = {
	args: {
		value: {
			startedAfter: new Date(2026, 7, 11, 23, 59, 59),
			startedBefore: new Date(2026, 7, 13, 10, 0, 0),
		},
	},
};

export const SameMonthLabel: Story = {
	args: {
		value: {
			startedAfter: new Date(2026, 3, 17),
			startedBefore: new Date(2026, 3, 19),
		},
	},
};

export const OpenPrefillsExpressions: Story = {
	args: {
		value: singleDayValue,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("button", { name: "Filter by time range" }),
		);

		// Committed bounds are shown as absolute local expressions.
		const fromInput = await body.findByLabelText("Start of time range");
		const toInput = body.getByLabelText("End of time range");
		expect(fromInput).toHaveValue("2026-04-10 07:23:00");
		expect(toInput).toHaveValue("2026-04-10 09:30:00");

		// The examples footer explains the accepted grammar.
		expect(body.getByText("Examples:")).toBeInTheDocument();
		expect(
			body.getByText("Defaults to midnight if no time is provided."),
		).toBeInTheDocument();

		// Apply stays disabled until the selection changes.
		expect(body.getByRole("button", { name: "Apply" })).toBeDisabled();
	},
};

export const OpenPrefillsNowForCurrentBoundary: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("button", { name: "Filter by time range" }),
		);

		// The default end boundary is the current moment, so it reads as
		// "now"; the start boundary is a frozen timestamp.
		const fromInput = await body.findByLabelText("Start of time range");
		const toInput = body.getByLabelText("End of time range");
		expect(fromInput).toHaveValue(formatDateTime(defaultValue.startedAfter));
		expect(toInput).toHaveValue("now");
	},
};

export const BlurNormalizesUnderspecifiedInput: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("button", { name: "Filter by time range" }),
		);

		// A clock-only expression resolves to the current day once the
		// input loses focus, so the user sees what will be applied.
		const fromInput = await body.findByLabelText("Start of time range");
		await userEvent.click(fromInput);
		await fireEvent.change(fromInput, { target: { value: "12:34" } });
		await userEvent.tab();
		await waitFor(() => {
			expect(fromInput).toHaveValue("2026-08-13 12:34:00");
		});

		// "now" is already unambiguous and stays untouched.
		const toInput = body.getByLabelText("End of time range");
		await userEvent.tab();
		expect(toInput).toHaveValue("now");
	},
};

export const BlurClampsReversedRange: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("button", { name: "Filter by time range" }),
		);

		// Blurring an out-of-order boundary snaps it back inside the range.
		const fromInput = await body.findByLabelText("Start of time range");
		const toInput = body.getByLabelText("End of time range");
		await userEvent.click(fromInput);
		await fireEvent.change(fromInput, {
			target: { value: "2026-08-13 16:00" },
		});
		await fireEvent.change(toInput, { target: { value: "2026-08-13 08:00" } });
		// Tabbing out of From clamps it below To; tabbing out of To then
		// reformats it without changing the now-valid range.
		await userEvent.tab();
		await userEvent.tab();
		await waitFor(() => {
			expect(fromInput).toHaveValue("2026-08-13 07:59:59");
		});
		expect(toInput).toHaveValue("2026-08-13 08:00:00");
		expect(body.queryByText("From must be before To")).toBeNull();
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

		const fromInput = await body.findByLabelText("Start of time range");
		await fireEvent.change(fromInput, {
			target: { value: "2026-08-13 08:00" },
		});

		const applyButton = body.getByRole("button", { name: "Apply" });
		expect(applyButton).toBeEnabled();
		await userEvent.click(applyButton);

		await waitFor(() => {
			expect(body.queryByRole("button", { name: "Apply" })).toBeNull();
		});
		expect(args.onChange).toHaveBeenCalledWith({
			startedAfter: new Date(2026, 7, 13, 8, 0, 0),
			startedBefore: fixedNow,
		});
	},
};

export const InvalidExpressionShowsError: Story = {
	args: {
		onChange: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("button", { name: "Filter by time range" }),
		);

		const fromInput = await body.findByLabelText("Start of time range");
		await fireEvent.change(fromInput, { target: { value: "30d" } });
		expect(fromInput).toHaveAttribute("aria-invalid", "true");
		expect(
			body.getByText("Enter a valid time, e.g. 2026-08-13 11:43"),
		).toBeInTheDocument();
		expect(body.getByRole("button", { name: "Apply" })).toBeDisabled();
	},
};

export const ReversedRangeShowsError: Story = {
	args: {
		onChange: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("button", { name: "Filter by time range" }),
		);

		// From after To is not a valid range and gets its own message.
		const fromInput = await body.findByLabelText("Start of time range");
		const toInput = body.getByLabelText("End of time range");
		await fireEvent.change(fromInput, {
			target: { value: "2026-08-13 16:00" },
		});
		await fireEvent.change(toInput, { target: { value: "2026-08-13 08:00" } });
		expect(body.getByText("From must be before To")).toBeInTheDocument();
		expect(body.getByRole("button", { name: "Apply" })).toBeDisabled();
	},
};

export const EscapeClosesWithoutApplying: Story = {
	args: {
		onChange: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("button", { name: "Filter by time range" }),
		);

		const fromInput = await body.findByLabelText("Start of time range");
		await fireEvent.change(fromInput, {
			target: { value: "2026-08-13 08:00" },
		});
		await userEvent.keyboard("{Escape}");

		await waitFor(() => {
			expect(body.queryByRole("button", { name: "Apply" })).toBeNull();
		});
		expect(args.onChange).not.toHaveBeenCalled();
	},
};
