import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("#/utils/timeZones", () => ({
	timeZones: ["America/New_York", "Europe/London", "UTC"],
}));

it("selects a timezone when Object.groupBy is unavailable", async () => {
	const groupBy = vi.spyOn(Object, "groupBy").mockImplementation(() => {
		throw new TypeError("Object.groupBy is not a function");
	});

	try {
		const { TimezoneCombobox } = await import("./TimezoneCombobox");
		const onValueChange = vi.fn();
		const user = userEvent.setup();
		render(
			<TimezoneCombobox
				id="timezone"
				value="UTC"
				onValueChange={onValueChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "UTC" }));
		await user.type(
			screen.getByRole("combobox", { name: "Search timezones" }),
			"new york",
		);
		await user.click(screen.getByRole("option", { name: "America/New York" }));

		expect(onValueChange).toHaveBeenCalledExactlyOnceWith("America/New_York");
	} finally {
		groupBy.mockRestore();
	}
});
