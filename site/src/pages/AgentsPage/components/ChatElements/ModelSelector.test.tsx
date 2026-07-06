import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { render } from "#/testHelpers/renderHelpers";
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

test("groups same-type models by provider instance", async () => {
	const user = userEvent.setup();
	render(
		<ModelSelector
			options={[
				{
					...MockModelSelectorOption,
					id: "anthropic-primary-sonnet",
					provider: "anthropic",
					providerId: "provider-anthropic-primary",
					providerLabel: "Anthropic",
					model: "claude-sonnet-4-20250514",
					displayName: "Claude Sonnet 4",
				},
				{
					...MockModelSelectorOption,
					id: "anthropic-hyper-opus",
					provider: "anthropic",
					providerId: "provider-anthropic-hyper",
					providerLabel: "Hyper",
					providerIcon: "/icon/coder.svg",
					model: "claude-opus-4-20250514",
					displayName: "Claude Opus 4",
				},
			]}
			value="anthropic-primary-sonnet"
			onValueChange={vi.fn()}
		/>,
	);

	await user.click(screen.getByRole("combobox"));
	const listbox = await screen.findByRole("listbox");

	expect(within(listbox).getByText("Anthropic")).toBeInTheDocument();
	expect(within(listbox).getByText("Hyper")).toBeInTheDocument();
	expect(within(listbox).getByText("Claude Sonnet 4")).toBeInTheDocument();
	expect(within(listbox).getByText("Claude Opus 4")).toBeInTheDocument();
});
