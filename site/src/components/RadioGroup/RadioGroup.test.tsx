import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { RadioGroup, RadioGroupItem } from "./RadioGroup";

describe("RadioGroup", () => {
	it("selects with arrow keys and skips disabled options", async () => {
		const user = userEvent.setup();
		const onValueChange = vi.fn();
		render(
			<RadioGroup
				defaultValue="first"
				onValueChange={onValueChange}
				aria-label="Options"
			>
				<RadioGroupItem value="first" aria-label="First" />
				<RadioGroupItem value="second" aria-label="Second" disabled />
				<RadioGroupItem value="third" aria-label="Third" />
			</RadioGroup>,
		);
		await user.tab();
		await user.keyboard("{ArrowDown}");
		expect(onValueChange).toHaveBeenCalledWith("third");
	});

	it("submits the selected value", async () => {
		const user = userEvent.setup();
		render(
			<form aria-label="Settings">
				<RadioGroup name="option" defaultValue="first">
					<RadioGroupItem value="first" aria-label="First" />
					<RadioGroupItem value="second" aria-label="Second" />
				</RadioGroup>
			</form>,
		);
		await user.click(screen.getByRole("radio", { name: "Second" }));
		expect(
			new FormData(
				screen.getByRole<HTMLFormElement>("form", { name: "Settings" }),
			).get("option"),
		).toBe("second");
	});
});
