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

test("renders selected model display name in the trigger", () => {
	render(
		<ModelSelector
			options={mockModelOptions}
			value="gpt-4o-mini"
			onValueChange={vi.fn()}
		/>,
	);

	const trigger = screen.getByRole("button", { name: /gpt-4o mini/i });
	expect(trigger).toBeInTheDocument();
});

test("renders badge-style trigger with pill styling", () => {
	render(
		<ModelSelector
			options={mockModelOptions}
			value="gpt-4o-mini"
			onValueChange={vi.fn()}
		/>,
	);

	const trigger = screen.getByRole("button", { name: /gpt-4o mini/i });
	expect(trigger.className).toContain("rounded-full");
	expect(trigger.className).toContain("bg-surface-secondary");
});

test("renders placeholder when no value is selected", () => {
	render(
		<ModelSelector
			options={mockModelOptions}
			value=""
			onValueChange={vi.fn()}
			placeholder="Pick a model"
		/>,
	);

	const trigger = screen.getByRole("button", { name: /pick a model/i });
	expect(trigger).toBeInTheDocument();
});

test("disables trigger when disabled prop is true", () => {
	render(
		<ModelSelector
			options={mockModelOptions}
			value="gpt-4o-mini"
			onValueChange={vi.fn()}
			disabled
		/>,
	);

	const trigger = screen.getByRole("button", { name: /gpt-4o mini/i });
	expect(trigger.className).toContain("pointer-events-none");
	expect(trigger.className).toContain("opacity-50");
});

test("disables trigger when options list is empty", () => {
	render(<ModelSelector options={[]} value="" onValueChange={vi.fn()} />);

	const trigger = screen.getByRole("button");
	expect(trigger.className).toContain("pointer-events-none");
});
