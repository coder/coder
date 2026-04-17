import { render, screen } from "@testing-library/react";
import { ModelSelector, type ModelSelectorOption } from "./ModelSelector";

const mockModelOptions: readonly ModelSelectorOption[] = [
	{
		id: "gpt-4o-mini",
		provider: "openai",
		model: "gpt-4o-mini",
		displayName: "GPT-4o mini",
	},
];

test("suppresses mouse-focus ring but keeps keyboard-focus ring on model selector trigger", () => {
	render(
		<ModelSelector
			options={mockModelOptions}
			value="gpt-4o-mini"
			onValueChange={vi.fn()}
		/>,
	);

	const trigger = screen.getByRole("combobox");

	// Mouse-focus ring should be suppressed.
	expect(trigger.className).toContain("focus:ring-0");
	// Keyboard-focus ring should remain.
	expect(trigger.className).toContain("focus-visible:ring-2");
	expect(trigger.className).not.toContain("focus-visible:ring-0");
});
