import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { formatContextLimit, ModelSelector } from "./ModelSelector";
import { MockModelSelectorOption } from "./modelSelectorFixtures";

describe("formatContextLimit", () => {
	it.each([
		{ tokens: 800, expected: "800" },
		{ tokens: 999.6, expected: "1K" },
		{ tokens: 128_000, expected: "128K" },
		{ tokens: 999_999, expected: "1.0M" },
		{ tokens: 1_000_000, expected: "1M" },
		{ tokens: 1_500_000, expected: "1.5M" },
	])("formats $tokens tokens as $expected", ({ tokens, expected }) => {
		expect(formatContextLimit(tokens)).toBe(expected);
	});
});

describe("ModelSelector", () => {
	it("points the trigger at the popup it opens", async () => {
		const user = userEvent.setup();
		renderComponent(
			<ModelSelector
				options={[MockModelSelectorOption]}
				value=""
				onValueChange={vi.fn()}
				placeholder="Select model"
			/>,
		);

		const trigger = screen.getByRole("combobox", { name: "Select model" });
		await user.click(trigger);
		const popup = await screen.findByRole("dialog");

		expect(trigger).toHaveAttribute("aria-controls", popup.id);
		expect(trigger).toHaveAttribute(
			"aria-haspopup",
			popup.getAttribute("role"),
		);
	});
});
