import type { ModelSelectorOption } from "./ModelSelector";

/**
 * Builds a ModelSelectorOption for tests and stories. Defaults to an OpenAI
 * option whose id and display name derive from the model identifier; pass
 * overrides for the fields a case cares about.
 */
export const makeModelSelectorOption = (
	overrides: Partial<ModelSelectorOption> = {},
): ModelSelectorOption => {
	const model = overrides.model ?? "gpt-4o";
	return {
		id: model,
		provider: "openai",
		model,
		displayName: model,
		...overrides,
	};
};
