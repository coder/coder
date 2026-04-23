import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import {
	AgentSettingsAgentsPageView,
	type AgentSettingsAgentsPageViewProps,
} from "./AgentSettingsAgentsPageView";

const OVERRIDE_MALFORMED_WARNING =
	"The saved override is malformed and is being treated as unset. Click Save to clear it.";
const UNAVAILABLE_SAVED_MODEL_WARNING =
	"The saved model is no longer enabled and will be ignored until you choose a new override.";
const COMPUTER_USE_COMPATIBILITY_WARNING =
	"The saved model uses the openai provider. Computer use currently keeps using the Anthropic default unless the override also uses Anthropic.";

const buildModelConfig = (
	overrides: Partial<TypesGen.ChatModelConfig>,
): TypesGen.ChatModelConfig => ({
	id: "model-default",
	provider: "openai",
	model: "gpt-4.1-mini",
	display_name: "GPT 4.1 Mini",
	enabled: true,
	is_default: false,
	context_limit: 1_000_000,
	compression_threshold: 70,
	created_at: "2026-03-12T12:00:00.000Z",
	updated_at: "2026-03-12T12:00:00.000Z",
	...overrides,
});

const buildOverrideData = (
	context: TypesGen.ChatAgentModelOverrideContext,
	overrides: Partial<TypesGen.ChatAgentModelOverrideResponse> = {},
): TypesGen.ChatAgentModelOverrideResponse => ({
	context,
	model_config_id: "",
	is_malformed: false,
	...overrides,
});

const generalModelConfig = buildModelConfig({
	id: "model-general-gpt-4.1-mini",
	display_name: "GPT 4.1 Mini",
});

const planSubagentModelConfig = buildModelConfig({
	id: "model-plan-gpt-4.1",
	model: "gpt-4.1",
	display_name: "GPT 4.1",
});

const computerUseCompatibleModelConfig = buildModelConfig({
	id: "model-computer-use-claude-sonnet-4",
	provider: "anthropic",
	model: "claude-sonnet-4",
	display_name: "Claude Sonnet 4",
	context_limit: 200_000,
});

const exploreFallbackModelConfig = buildModelConfig({
	id: "model-explore-blank-display",
	provider: "anthropic",
	model: "claude-sonnet-4-20250514",
	display_name: "",
	context_limit: 200_000,
});

const generalDisabledModelConfig = buildModelConfig({
	id: "model-general-disabled",
	model: "gpt-4.1-legacy",
	display_name: "GPT 4.1 Legacy",
	enabled: false,
});

const planDisabledModelConfig = buildModelConfig({
	id: "model-plan-disabled",
	model: "gpt-4.1-plan-legacy",
	display_name: "GPT 4.1 Plan Legacy",
	enabled: false,
});

const exploreDisabledModelConfig = buildModelConfig({
	id: "model-explore-disabled",
	provider: "anthropic",
	model: "claude-haiku-legacy",
	display_name: "Claude Haiku Legacy",
	enabled: false,
	context_limit: 200_000,
});

const computerUseDisabledModelConfig = buildModelConfig({
	id: "model-computer-use-disabled",
	provider: "anthropic",
	model: "claude-opus-legacy",
	display_name: "Claude Opus Legacy",
	enabled: false,
	context_limit: 200_000,
});

const allModelConfigs: TypesGen.ChatModelConfig[] = [
	generalModelConfig,
	planSubagentModelConfig,
	computerUseCompatibleModelConfig,
	exploreFallbackModelConfig,
	generalDisabledModelConfig,
	planDisabledModelConfig,
	exploreDisabledModelConfig,
	computerUseDisabledModelConfig,
];

const makeArgs = (
	overrides: Partial<AgentSettingsAgentsPageViewProps> = {},
): AgentSettingsAgentsPageViewProps => ({
	generalModelOverrideData: buildOverrideData("general"),
	planSubagentModelOverrideData: buildOverrideData("plan_subagent"),
	exploreModelOverrideData: buildOverrideData("explore"),
	computerUseModelOverrideData: buildOverrideData("computer_use"),
	modelConfigsData: allModelConfigs,
	modelConfigsError: undefined,
	isLoadingModelConfigs: false,
	onSaveGeneralModelOverride: fn(),
	isSavingGeneralModelOverride: false,
	isSaveGeneralModelOverrideError: false,
	onSavePlanSubagentModelOverride: fn(),
	isSavingPlanSubagentModelOverride: false,
	isSavePlanSubagentModelOverrideError: false,
	onSaveExploreModelOverride: fn(),
	isSavingExploreModelOverride: false,
	isSaveExploreModelOverrideError: false,
	onSaveComputerUseModelOverride: fn(),
	isSavingComputerUseModelOverride: false,
	isSaveComputerUseModelOverrideError: false,
	...overrides,
});

const getSection = async (
	canvasElement: HTMLElement,
	headingName: string,
): Promise<HTMLElement> => {
	const canvas = within(canvasElement);
	const heading = await canvas.findByRole("heading", { name: headingName });
	const section = heading.closest("section");
	if (!(section instanceof HTMLElement)) {
		throw new Error(
			`Expected ${headingName} heading to live inside a section.`,
		);
	}
	return section;
};

const selectModelInSection = async (
	section: HTMLElement,
	canvasElement: HTMLElement,
	currentSelectionName: string | RegExp,
	optionName: string,
) => {
	const trigger = within(section).getByRole("combobox", {
		name: currentSelectionName,
	});
	await userEvent.click(trigger);
	const body = within(canvasElement.ownerDocument.body);
	await userEvent.click(await body.findByRole("option", { name: optionName }));
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsAgentsPageView",
	component: AgentSettingsAgentsPageView,
	args: makeArgs(),
} satisfies Meta<typeof AgentSettingsAgentsPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsAgentsPageView>;

export const AllOverridesUnset: Story = {
	args: makeArgs(),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Agents");

		const headings = await canvas.findAllByRole("heading", { level: 3 });
		expect(headings.map((heading) => heading.textContent?.trim())).toEqual([
			"General model",
			"Plan subagent model",
			"Explore subagent model",
			"Computer use subagent model",
		]);

		for (const headingName of [
			"General model",
			"Plan subagent model",
			"Explore subagent model",
			"Computer use subagent model",
		]) {
			const section = await getSection(canvasElement, headingName);
			expect(
				within(section).getByRole("combobox", { name: "Use chat default" }),
			).toBeInTheDocument();
			expect(
				within(section).getByRole("button", { name: "Save" }),
			).toBeDisabled();
		}
	},
};

export const EachOverrideTargetsItsOwnMutation: Story = {
	args: makeArgs({
		generalModelOverrideData: buildOverrideData("general", {
			model_config_id: generalModelConfig.id,
		}),
		planSubagentModelOverrideData: buildOverrideData("plan_subagent", {
			model_config_id: planSubagentModelConfig.id,
		}),
		exploreModelOverrideData: buildOverrideData("explore", {
			model_config_id: exploreFallbackModelConfig.id,
		}),
		computerUseModelOverrideData: buildOverrideData("computer_use", {
			model_config_id: computerUseCompatibleModelConfig.id,
			is_effective: true,
		}),
	}),
	play: async ({ canvasElement, args }) => {
		const generalSection = await getSection(canvasElement, "General model");
		const planSection = await getSection(canvasElement, "Plan subagent model");
		const exploreSection = await getSection(
			canvasElement,
			"Explore subagent model",
		);
		const computerUseSection = await getSection(
			canvasElement,
			"Computer use subagent model",
		);

		await selectModelInSection(
			generalSection,
			canvasElement,
			/gpt 4\.1 mini/i,
			"Claude Sonnet 4",
		);
		const generalSaveButton = within(generalSection).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(generalSaveButton).toBeEnabled();
		});
		await userEvent.click(generalSaveButton);
		await waitFor(() => {
			expect(args.onSaveGeneralModelOverride).toHaveBeenCalledWith(
				{ model_config_id: computerUseCompatibleModelConfig.id },
				expect.anything(),
			);
		});

		const planClearButton = within(planSection).getByRole("button", {
			name: "Clear",
		});
		await userEvent.click(planClearButton);
		const planSaveButton = within(planSection).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(planSaveButton).toBeEnabled();
		});
		await userEvent.click(planSaveButton);
		await waitFor(() => {
			expect(args.onSavePlanSubagentModelOverride).toHaveBeenCalledWith(
				{ model_config_id: "" },
				expect.anything(),
			);
		});

		expect(
			within(exploreSection).getByRole("combobox", {
				name: /claude-sonnet-4-20250514/i,
			}),
		).toHaveTextContent("claude-sonnet-4-20250514");
		await selectModelInSection(
			exploreSection,
			canvasElement,
			/claude-sonnet-4-20250514/i,
			"GPT 4.1",
		);
		const exploreSaveButton = within(exploreSection).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(exploreSaveButton).toBeEnabled();
		});
		await userEvent.click(exploreSaveButton);
		await waitFor(() => {
			expect(args.onSaveExploreModelOverride).toHaveBeenCalledWith(
				{ model_config_id: planSubagentModelConfig.id },
				expect.anything(),
			);
		});

		await selectModelInSection(
			computerUseSection,
			canvasElement,
			/claude sonnet 4/i,
			"GPT 4.1 Mini",
		);
		const computerUseSaveButton = within(computerUseSection).getByRole(
			"button",
			{
				name: "Save",
			},
		);
		await waitFor(() => {
			expect(computerUseSaveButton).toBeEnabled();
		});
		await userEvent.click(computerUseSaveButton);
		await waitFor(() => {
			expect(args.onSaveComputerUseModelOverride).toHaveBeenCalledWith(
				{ model_config_id: generalModelConfig.id },
				expect.anything(),
			);
		});
	},
};

export const MalformedOverridesRemainClearableAndSaveable: Story = {
	args: makeArgs({
		generalModelOverrideData: buildOverrideData("general", {
			is_malformed: true,
		}),
		planSubagentModelOverrideData: buildOverrideData("plan_subagent", {
			is_malformed: true,
		}),
		exploreModelOverrideData: buildOverrideData("explore", {
			is_malformed: true,
		}),
		computerUseModelOverrideData: buildOverrideData("computer_use", {
			is_malformed: true,
		}),
	}),
	play: async ({ canvasElement, args }) => {
		const generalSection = await getSection(canvasElement, "General model");
		const planSection = await getSection(canvasElement, "Plan subagent model");
		const exploreSection = await getSection(
			canvasElement,
			"Explore subagent model",
		);
		const computerUseSection = await getSection(
			canvasElement,
			"Computer use subagent model",
		);

		for (const section of [
			generalSection,
			planSection,
			exploreSection,
			computerUseSection,
		]) {
			await within(section).findByText(OVERRIDE_MALFORMED_WARNING);
		}

		const planSaveButton = within(planSection).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(planSaveButton).toBeEnabled();
		});
		await userEvent.click(planSaveButton);
		await waitFor(() => {
			expect(args.onSavePlanSubagentModelOverride).toHaveBeenCalledWith(
				{ model_config_id: "" },
				expect.anything(),
			);
		});

		const computerUseSaveButton = within(computerUseSection).getByRole(
			"button",
			{
				name: "Save",
			},
		);
		await waitFor(() => {
			expect(computerUseSaveButton).toBeEnabled();
		});
		await userEvent.click(computerUseSaveButton);
		await waitFor(() => {
			expect(args.onSaveComputerUseModelOverride).toHaveBeenCalledWith(
				{ model_config_id: "" },
				expect.anything(),
			);
		});
	},
};

export const UnavailableSavedModels: Story = {
	args: makeArgs({
		generalModelOverrideData: buildOverrideData("general", {
			model_config_id: generalDisabledModelConfig.id,
		}),
		planSubagentModelOverrideData: buildOverrideData("plan_subagent", {
			model_config_id: planDisabledModelConfig.id,
		}),
		exploreModelOverrideData: buildOverrideData("explore", {
			model_config_id: exploreDisabledModelConfig.id,
		}),
		computerUseModelOverrideData: buildOverrideData("computer_use", {
			model_config_id: computerUseDisabledModelConfig.id,
		}),
	}),
	play: async ({ canvasElement }) => {
		const generalSection = await getSection(canvasElement, "General model");
		const planSection = await getSection(canvasElement, "Plan subagent model");
		const exploreSection = await getSection(
			canvasElement,
			"Explore subagent model",
		);
		const computerUseSection = await getSection(
			canvasElement,
			"Computer use subagent model",
		);

		for (const section of [
			generalSection,
			planSection,
			exploreSection,
			computerUseSection,
		]) {
			await within(section).findByText(UNAVAILABLE_SAVED_MODEL_WARNING);
			expect(
				within(section).getByRole("combobox", { name: "Unavailable model" }),
			).toBeInTheDocument();
		}
	},
};

export const ComputerUseCompatibilityWarning: Story = {
	args: makeArgs({
		computerUseModelOverrideData: buildOverrideData("computer_use", {
			model_config_id: generalModelConfig.id,
			is_effective: false,
			ignored_reason: COMPUTER_USE_COMPATIBILITY_WARNING,
		}),
	}),
	play: async ({ canvasElement }) => {
		const computerUseSection = await getSection(
			canvasElement,
			"Computer use subagent model",
		);

		await within(computerUseSection).findByText(
			COMPUTER_USE_COMPATIBILITY_WARNING,
		);
		expect(
			within(computerUseSection).getByRole("combobox", {
				name: /gpt 4\.1 mini/i,
			}),
		).toBeInTheDocument();
	},
};
