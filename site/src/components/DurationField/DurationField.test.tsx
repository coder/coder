import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DurationField } from "./DurationField";

describe("DurationField", () => {
	it("should convert hours to days correctly when switching units", async () => {
		const user = userEvent.setup();
		const mockOnChange = jest.fn();

		// Start with 26 hours (which should round up to 2 days)
		const twentySixHoursInMs = 26 * 60 * 60 * 1000;

		render(
			<DurationField valueMs={twentySixHoursInMs} onChange={mockOnChange} />,
		);

		// Component should initially display in hours since 26 hours is not an integer number of days
		const input = screen.getByRole("textbox");
		expect(input).toHaveValue("26");

		// Switch to days by changing the select
		const select = screen.getByRole("combobox");
		await user.click(select);

		// Find and click the Days option
		const daysOption = screen.getByRole("option", { name: "Days" });
		await user.click(daysOption);

		// This is the key test - when switching from 26 hours to days,
		// it should round up to 2 days and call onChange with the correct value
		expect(mockOnChange).toHaveBeenCalledWith(2 * 24 * 60 * 60 * 1000);
	});

	it("should convert days to hours correctly when switching units", async () => {
		const user = userEvent.setup();
		const mockOnChange = jest.fn();

		// Start with 2 days (exact days should display as days initially)
		const twoDaysInMs = 2 * 24 * 60 * 60 * 1000;

		render(<DurationField valueMs={twoDaysInMs} onChange={mockOnChange} />);

		// Should initially show 2 in the input
		const input = screen.getByRole("textbox");
		expect(input).toHaveValue("2");

		// Switch to hours
		const select = screen.getByRole("combobox");
		await user.click(select);

		const hoursOption = screen.getByRole("option", { name: "Hours" });
		await user.click(hoursOption);

		// Should have called onChange with the value in hours based on the field value
		// The new logic takes the current field value (2) and converts it to hours
		expect(mockOnChange).toHaveBeenCalledWith(2 * 60 * 60 * 1000);
	});

	// Test the specific bug that was fixed: 26 hours → 2 days conversion
	it("should properly handle the original bug case: 26 hours to days", async () => {
		const user = userEvent.setup();
		const mockOnChange = jest.fn();

		const twentySixHoursInMs = 26 * 60 * 60 * 1000;

		render(
			<DurationField valueMs={twentySixHoursInMs} onChange={mockOnChange} />,
		);

		const select = screen.getByRole("combobox");
		await user.click(select);
		await user.click(screen.getByRole("option", { name: "Days" }));

		// The key fix: should calculate days based on the original millisecond value,
		// not the current field value. 26 hours = 1.083... days, rounded up = 2 days
		const twoDaysInMs = 2 * 24 * 60 * 60 * 1000;
		expect(mockOnChange).toHaveBeenCalledWith(twoDaysInMs);
	});
});
