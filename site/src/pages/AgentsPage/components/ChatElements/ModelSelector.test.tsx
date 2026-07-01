import { render, screen } from "@testing-library/react";
import { ModelSelector, type ModelSelectorOption } from "./ModelSelector";
import { MockModelSelectorOption } from "./modelSelectorFixtures";

const mockModelOptions: readonly ModelSelectorOption[] = [
	{
		...MockModelSelectorOption,
		id: "gpt-4o-mini",
		model: "gpt-4o-mini",
		displayName: "GPT-4o mini",
	},
	{
		...MockModelSelectorOption,
		id: "claude-opus",
		provider: "anthropic",
		model: "claude-opus-4-1",
		displayName: "Claude Opus 4.1",
		contextLimit: 1_000_000,
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
