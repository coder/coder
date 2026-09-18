import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
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
