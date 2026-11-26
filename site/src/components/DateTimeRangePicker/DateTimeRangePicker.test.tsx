import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { DateTimeRangePicker } from "./DateTimeRangePicker";

it("does not apply a custom range longer than maxDays", async () => {
	const user = userEvent.setup();
	const onChange = vi.fn();
	const now = new Date(2026, 3, 16, 10, 30);
	render(
		<DateTimeRangePicker
			now={now}
			maxDays={7}
			value={{
				start: new Date(2026, 3, 15, 10, 30),
				end: now,
				preset: "last_24h",
			}}
			onChange={onChange}
		/>,
	);

	await user.click(screen.getByRole("combobox", { name: "Time range" }));
	await user.click(await screen.findByRole("radio", { name: "Custom range" }));
	await user.click(screen.getByRole("button", { name: /April 1st, 2026/ }));
	await user.click(screen.getByRole("button", { name: /April 9th, 2026/ }));
	await user.click(screen.getByRole("button", { name: "Apply" }));

	expect(onChange).not.toHaveBeenCalled();
});

it("applies a start on the minDate day only once it is past the cutoff", async () => {
	const user = userEvent.setup();
	const onChange = vi.fn();
	const now = new Date(2026, 3, 16, 10, 30);
	render(
		<DateTimeRangePicker
			now={now}
			minDate={new Date(2026, 3, 10, 9, 0)}
			value={{
				start: new Date(2026, 3, 15, 10, 30),
				end: now,
				preset: "last_24h",
			}}
			onChange={onChange}
		/>,
	);

	await user.click(screen.getByRole("combobox", { name: "Time range" }));
	await user.click(await screen.findByRole("radio", { name: "Custom range" }));
	await user.click(screen.getByRole("button", { name: /April 10th, 2026/ }));
	await user.click(screen.getByRole("button", { name: "Apply" }));
	expect(onChange).not.toHaveBeenCalled();

	const from = screen.getByLabelText("From");
	await user.clear(from);
	await user.type(from, "09:00:00");
	await user.click(screen.getByRole("button", { name: "Apply" }));
	expect(onChange).toHaveBeenCalledWith({
		start: new Date(2026, 3, 10, 9, 0),
		end: new Date(2026, 3, 10, 23, 59, 59),
	});
});

it("picks a quick range with the keyboard alone", async () => {
	const user = userEvent.setup();
	const onChange = vi.fn();
	const now = new Date(2026, 3, 16, 10, 30);
	render(
		<DateTimeRangePicker
			now={now}
			value={{
				start: new Date(2026, 3, 15, 10, 30),
				end: now,
				preset: "last_24h",
			}}
			onChange={onChange}
		/>,
	);

	await user.tab();
	const trigger = screen.getByRole("combobox", { name: "Time range" });
	await user.keyboard("{ArrowDown}");
	expect(
		await screen.findByRole("radio", { name: "Last 24 hours" }),
	).toHaveFocus();
	expect(screen.getByRole("dialog", { name: "Time range" })).toHaveAttribute(
		"id",
		trigger.getAttribute("aria-controls"),
	);
	await user.keyboard("{ArrowDown}{Enter}");
	expect(onChange).toHaveBeenCalledWith(
		expect.objectContaining({ preset: "today" }),
	);
});

it("opens with ArrowUp", async () => {
	const user = userEvent.setup();
	const now = new Date(2026, 3, 16, 10, 30);
	render(
		<DateTimeRangePicker
			now={now}
			value={{
				start: new Date(2026, 3, 15, 10, 30),
				end: now,
				preset: "last_24h",
			}}
			onChange={vi.fn()}
		/>,
	);

	await user.tab();
	await user.keyboard("{ArrowUp}");
	expect(
		await screen.findByRole("radio", { name: "Last 24 hours" }),
	).toHaveFocus();
});

const lastSeenNow = new Date(2026, 3, 16, 10, 30);
const lastSeenPresets = [
	{
		id: "all_time",
		label: "All time",
		placeholder: "All time",
		range: () => ({ start: new Date(0), end: lastSeenNow }),
	},
	{
		id: "last_7d",
		label: "Last 7 days",
		range: () => ({ start: new Date(2026, 3, 9, 10, 30), end: lastSeenNow }),
	},
];

const renderLastSeenPicker = (preset: string) =>
	render(
		<DateTimeRangePicker
			now={lastSeenNow}
			presets={lastSeenPresets}
			label="Last seen"
			showLabel
			value={{ ...lastSeenPresets[0].range(), preset }}
			onChange={vi.fn()}
		/>,
	);

it("names the labelled trigger with its placeholder value", () => {
	renderLastSeenPicker("all_time");
	expect(screen.getByRole("combobox")).toHaveAccessibleName(
		"Last seen: All time",
	);
});

it("names the labelled trigger with its picked value", () => {
	renderLastSeenPicker("last_7d");
	expect(screen.getByRole("combobox")).toHaveAccessibleName(
		"Last seen: Last 7 days",
	);
});
