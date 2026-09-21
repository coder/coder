import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DateRangePicker } from "./DateRangePicker";

it("restarts the calendar selection when a range exceeds maxDays", async () => {
	const user = userEvent.setup();
	const onChange = vi.fn();
	const now = new Date(2025, 2, 15, 12);
	render(
		<DateRangePicker
			now={now}
			maxDays={7}
			value={{ startDate: new Date(2025, 2, 1), endDate: new Date(2025, 2, 3) }}
			onChange={onChange}
		/>,
	);

	await user.click(screen.getByRole("button", { name: /Mar 1, 2025/ }));
	await user.click(
		await screen.findByRole("button", { name: /March 12th, 2025/ }),
	);
	await user.click(screen.getByRole("button", { name: /March 14th, 2025/ }));
	await user.click(screen.getByRole("button", { name: "Apply" }));

	expect(onChange).toHaveBeenCalledTimes(1);
	expect(onChange).toHaveBeenCalledWith({
		startDate: new Date(2025, 2, 12),
		endDate: new Date(2025, 2, 15),
	});
});

describe("maxDays across a daylight-saving transition", () => {
	afterEach(() => {
		vi.unstubAllEnvs();
	});

	// Dates are built inside the test so they use the stubbed zone.
	it.each([
		{
			name: "restarts on the day that would run an hour past the limit",
			tz: "America/New_York",
			startMonth: 9,
			trigger: /Oct 10, 2025/,
			lastDay: /November 9th, 2025/,
			commits: false,
		},
		{
			name: "restarts on the day that would run half an hour past the limit",
			tz: "Australia/Lord_Howe",
			startMonth: 2,
			trigger: /Mar 10, 2025/,
			lastDay: /April 9th, 2025/,
			commits: false,
		},
		{
			name: "allows the full count in standard time",
			tz: "America/New_York",
			startMonth: 10,
			trigger: /Nov 10, 2025/,
			lastDay: /December 10th, 2025/,
			commits: true,
		},
	])("$name", async ({ tz, startMonth, trigger, lastDay, commits }) => {
		vi.stubEnv("TZ", tz);
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<DateRangePicker
				now={new Date(2025, startMonth + 1, 15, 12)}
				maxDays={31}
				value={{
					startDate: new Date(2025, startMonth, 10),
					endDate: new Date(2025, startMonth, 11),
				}}
				onChange={onChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: trigger }));
		await user.click(await screen.findByRole("button", { name: lastDay }));
		await user.click(screen.getByRole("button", { name: "Apply" }));

		if (commits) {
			expect(onChange).toHaveBeenCalledWith({
				startDate: new Date(2025, startMonth, 10),
				endDate: new Date(2025, startMonth + 1, 11),
			});
		} else {
			expect(onChange).not.toHaveBeenCalled();
		}
	});

	it("restarts a backward extension that would run an hour past the limit", async () => {
		vi.stubEnv("TZ", "America/New_York");
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<DateRangePicker
				now={new Date(2026, 10, 15, 12)}
				maxDays={31}
				value={{
					startDate: new Date(2026, 10, 10),
					endDate: new Date(2026, 10, 12),
				}}
				onChange={onChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: /Nov 10, 2026/ }));
		await user.click(
			await screen.findByRole("button", { name: "Go to the Previous Month" }),
		);
		await user.click(
			await screen.findByRole("button", { name: /October 12th, 2026/ }),
		);
		await user.click(screen.getByRole("button", { name: "Apply" }));
		expect(onChange).not.toHaveBeenCalled();

		await user.click(
			screen.getByRole("button", { name: /November 10th, 2026/ }),
		);
		await user.click(screen.getByRole("button", { name: "Apply" }));
		expect(onChange).toHaveBeenCalledWith({
			startDate: new Date(2026, 9, 12),
			endDate: new Date(2026, 10, 11),
		});
	});
});

it("ignores days before minDate", async () => {
	const user = userEvent.setup();
	const onChange = vi.fn();
	const now = new Date(2025, 2, 15, 12);
	render(
		<DateRangePicker
			now={now}
			minDate={new Date(2025, 2, 10)}
			value={{
				startDate: new Date(2025, 2, 12),
				endDate: new Date(2025, 2, 14),
			}}
			onChange={onChange}
		/>,
	);

	await user.click(screen.getByRole("button", { name: /Mar 12, 2025/ }));
	await user.click(
		await screen.findByRole("button", { name: /March 9th, 2025/ }),
	);
	await user.click(screen.getByRole("button", { name: "Apply" }));
	expect(onChange).not.toHaveBeenCalled();

	await user.click(screen.getByRole("button", { name: /March 10th, 2025/ }));
	await user.click(screen.getByRole("button", { name: "Apply" }));
	expect(onChange).toHaveBeenCalledTimes(1);
	expect(onChange).toHaveBeenCalledWith(
		expect.objectContaining({ startDate: new Date(2025, 2, 10) }),
	);
});

it.each([
	{ name: "without a retention cutoff", minDate: undefined },
	{ name: "with a retention cutoff", minDate: new Date(2025, 2, 10) },
])("allows today but ignores future dates $name", async ({ minDate }) => {
	const user = userEvent.setup();
	const onChange = vi.fn();
	const now = new Date(2025, 2, 15, 12);
	render(
		<DateRangePicker
			now={now}
			minDate={minDate}
			value={{
				startDate: new Date(2025, 2, 12),
				endDate: new Date(2025, 2, 14),
			}}
			onChange={onChange}
		/>,
	);
	await user.click(screen.getByRole("button", { name: /Mar 12, 2025/ }));
	await user.click(
		await screen.findByRole("button", { name: /March 16th, 2025/ }),
	);
	await user.click(screen.getByRole("button", { name: "Apply" }));
	expect(onChange).not.toHaveBeenCalled();
	await user.click(screen.getByRole("button", { name: /March 15th, 2025/ }));
	await user.click(screen.getByRole("button", { name: "Apply" }));
	expect(onChange).toHaveBeenCalledOnce();
	expect(onChange).toHaveBeenCalledWith(
		expect.objectContaining({ endDate: new Date(2025, 2, 15, 13) }),
	);
});

it("keeps a single day selected when maxDays is 1", async () => {
	const user = userEvent.setup();
	const onChange = vi.fn();
	render(
		<DateRangePicker
			now={new Date(2025, 2, 15, 12)}
			maxDays={1}
			value={{
				startDate: new Date(2025, 2, 12),
				endDate: new Date(2025, 2, 13),
			}}
			onChange={onChange}
		/>,
	);

	await user.click(screen.getByRole("button", { name: /Mar 12, 2025/ }));
	await user.click(
		await screen.findByRole("button", { name: /March 10th, 2025/ }),
	);
	await user.click(screen.getByRole("button", { name: /March 11th, 2025/ }));
	await user.click(screen.getByRole("button", { name: "Apply" }));

	expect(onChange).toHaveBeenCalledTimes(1);
	expect(onChange).toHaveBeenCalledWith({
		startDate: new Date(2025, 2, 11),
		endDate: new Date(2025, 2, 12),
	});
});

it("only offers presets that fit within maxDays", async () => {
	const user = userEvent.setup();
	const onChange = vi.fn();
	const now = new Date(2025, 2, 15, 12);
	render(
		<DateRangePicker
			now={now}
			maxDays={7}
			value={{
				startDate: new Date(2025, 2, 12),
				endDate: new Date(2025, 2, 14),
			}}
			onChange={onChange}
		/>,
	);
	const trigger = screen.getByRole("button", { name: /Mar 12, 2025/ });

	await user.click(trigger);
	const offered = screen
		.getAllByRole("button", { name: /^(Today|Yesterday|Last \d+ days)$/ })
		.map((preset) => preset.textContent ?? "");
	for (const label of offered) {
		await user.click(screen.getByRole("button", { name: label }));
		await user.click(trigger);
	}
	expect(onChange).toHaveBeenCalled();
	expect(onChange).toHaveBeenCalledTimes(offered.length);
	for (const [range] of onChange.mock.calls) {
		expect(
			range.endDate.getTime() - range.startDate.getTime(),
		).toBeLessThanOrEqual(7 * 24 * 60 * 60 * 1000);
	}
});

it("excludes the day a minDate cutoff falls inside of", async () => {
	const user = userEvent.setup();
	const onChange = vi.fn();
	const minDate = new Date(2025, 2, 10, 15);
	render(
		<DateRangePicker
			now={new Date(2025, 2, 16, 18)}
			minDate={minDate}
			value={{
				startDate: new Date(2025, 2, 12),
				endDate: new Date(2025, 2, 14),
			}}
			onChange={onChange}
		/>,
	);
	const trigger = screen.getByRole("button", { name: /Mar 12, 2025/ });

	// "Last 7 days" starts at 18:00 on the cutoff day and must not be offered,
	// since its committed start would be that day's midnight.
	await user.click(trigger);
	const offered = screen
		.getAllByRole("button", { name: /^(Today|Yesterday|Last \d+ days)$/ })
		.map((preset) => preset.textContent ?? "");
	for (const label of offered) {
		await user.click(screen.getByRole("button", { name: label }));
		await user.click(trigger);
	}
	expect(onChange).toHaveBeenCalled();
	expect(onChange).toHaveBeenCalledTimes(offered.length);
	for (const [range] of onChange.mock.calls) {
		expect(range.startDate.getTime()).toBeGreaterThanOrEqual(minDate.getTime());
	}
	onChange.mockClear();

	await user.click(screen.getByRole("button", { name: /March 10th, 2025/ }));
	await user.click(screen.getByRole("button", { name: "Apply" }));
	expect(onChange).not.toHaveBeenCalled();
	await user.click(screen.getByRole("button", { name: /March 11th, 2025/ }));
	await user.click(screen.getByRole("button", { name: "Apply" }));
	expect(onChange).toHaveBeenCalledWith(
		expect.objectContaining({ startDate: new Date(2025, 2, 11) }),
	);
});

it("stays closed after being disabled and re-enabled while open", async () => {
	const user = userEvent.setup();
	const onChange = vi.fn();
	const props = {
		now: new Date(2025, 2, 15, 12),
		value: { startDate: new Date(2025, 2, 12), endDate: new Date(2025, 2, 14) },
		onChange,
	};
	const { rerender } = render(<DateRangePicker {...props} />);
	const trigger = screen.getByRole("button", { name: /Mar 12, 2025/ });
	await user.click(trigger);
	await screen.findByRole("button", { name: "Apply" });

	rerender(<DateRangePicker {...props} disabled />);
	rerender(<DateRangePicker {...props} />);

	// The click opens a closed picker; it would close one that reopened by itself.
	await user.click(trigger);
	await user.click(
		await screen.findByRole("button", { name: /March 10th, 2025/ }),
	);
	await user.click(screen.getByRole("button", { name: /March 11th, 2025/ }));
	await user.click(screen.getByRole("button", { name: "Apply" }));
	expect(onChange).toHaveBeenCalledWith({
		startDate: new Date(2025, 2, 10),
		endDate: new Date(2025, 2, 12),
	});
});
