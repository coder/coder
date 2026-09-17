import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { render } from "#/testHelpers/renderHelpers";
import { DateRangePicker } from "./DateRangePicker";

it("resets an overlong calendar selection before committing a new range", async () => {
	const user = userEvent.setup();
	const onChange = vi.fn();
	render(
		<DateRangePicker
			value={{
				startDate: new Date("2025-03-01T00:00:00Z"),
				endDate: new Date("2025-03-03T00:00:00Z"),
			}}
			onChange={onChange}
			now={new Date("2025-03-20T12:00:00Z")}
			maxDays={7}
		/>,
	);

	await user.click(screen.getByRole("button", { name: /Mar 1, 2025/ }));
	await user.click(
		screen.getByRole("button", { name: "Wednesday, March 5th, 2025" }),
	);
	await user.click(
		screen.getByRole("button", { name: "Saturday, March 15th, 2025" }),
	);
	await user.click(screen.getByRole("button", { name: "Apply" }));

	expect(onChange).not.toHaveBeenCalled();
});
