import { render, screen } from "@testing-library/react";
import { ModelSelector, type ModelSelectorOption } from "./model-selector";

const mockModelOptions: readonly ModelSelectorOption[] = [
	{
		id: "gpt-4o-mini",
		provider: "openai",
		model: "gpt-4o-mini",
		displayName: "GPT-4o mini",
	},
];

test("does not suppress focus ring styles on the model selector trigger", () => {
	render(
		<ModelSelector
			options={mockModelOptions}
			value="gpt-4o-mini"
			onValueChange={vi.fn()}
		/>,
	);

	const trigger = screen.getByRole("combobox");

	expect(trigger.className).not.toContain("focus:ring-0");
	expect(trigger.className).not.toContain("focus-visible:ring-0");
});
