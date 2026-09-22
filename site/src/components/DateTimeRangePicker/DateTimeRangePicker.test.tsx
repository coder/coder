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

	await user.click(screen.getByRole("button", { name: "Last 24 hours" }));
	await user.click(await screen.findByRole("radio", { name: "Custom range" }));
	await user.click(screen.getByRole("button", { name: /April 1st, 2026/ }));
	await user.click(screen.getByRole("button", { name: /April 9th, 2026/ }));
	await user.click(screen.getByRole("button", { name: "Apply" }));

	expect(onChange).not.toHaveBeenCalled();
});
