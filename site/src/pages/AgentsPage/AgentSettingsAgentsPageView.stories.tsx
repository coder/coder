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

const claudeSonnetModelConfig = buildModelConfig({
	id: "model-claude-sonnet-4",
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

const exploreDisabledModelConfig = buildModelConfig({
	id: "model-explore-disabled",
	provider: "anthropic",
	model: "claude-haiku-legacy",
	display_name: "Claude Haiku Legacy",
	enabled: false,
	context_limit: 200_000,
});

const allModelConfigs: TypesGen.ChatModelConfig[] = [
	generalModelConfig,
	claudeSonnetModelConfig,
	exploreFallbackModelConfig,
	generalDisabledModelConfig,
	exploreDisabledModelConfig,
];

const makeArgs = (
	overrides: Partial<AgentSettingsAgentsPageViewProps> = {},
): AgentSettingsAgentsPageViewProps => ({
	generalModelOverrideData: buildOverrideData("general"),
	exploreModelOverrideData: buildOverrideData("explore"),
	modelConfigsData: allModelConfigs,
	modelConfigsError: undefined,
	isLoadingModelConfigs: false,
	onSaveGeneralModelOverride: fn(),
	isSavingGeneralModelOverride: false,
	isSaveGeneralModelOverrideError: false,
	onSaveExploreModelOverride: fn(),
	isSavingExploreModelOverride: false,
	isSaveExploreModelOverrideError: false,
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
			"Explore subagent model",
		]);

		for (const headingName of ["General model", "Explore subagent model"]) {
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

export const EachOverrideSetToEnabledModel: Story = {
	args: makeArgs({
		generalModelOverrideData: buildOverrideData("general", {
			model_config_id: generalModelConfig.id,
		}),
		exploreModelOverrideData: buildOverrideData("explore", {
			model_config_id: exploreFallbackModelConfig.id,
		}),
	}),
	play: async ({ canvasElement, args }) => {
		const generalSection = await getSection(canvasElement, "General model");
		const exploreSection = await getSection(
			canvasElement,
			"Explore subagent model",
		);

		expect(
			within(exploreSection).getByRole("combobox", {
				name: /claude-sonnet-4-20250514/i,
			}),
		).toHaveTextContent("claude-sonnet-4-20250514");

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
				{ model_config_id: claudeSonnetModelConfig.id },
				expect.anything(),
			);
		});

		const exploreClearButton = within(exploreSection).getByRole("button", {
			name: "Clear",
		});
		await userEvent.click(exploreClearButton);
		const exploreSaveButton = within(exploreSection).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(exploreSaveButton).toBeEnabled();
		});
		await userEvent.click(exploreSaveButton);
		await waitFor(() => {
			expect(args.onSaveExploreModelOverride).toHaveBeenCalledWith(
				{ model_config_id: "" },
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
		exploreModelOverrideData: buildOverrideData("explore", {
			is_malformed: true,
		}),
	}),
	play: async ({ canvasElement, args }) => {
		const generalSection = await getSection(canvasElement, "General model");
		const exploreSection = await getSection(
			canvasElement,
			"Explore subagent model",
		);

		for (const section of [generalSection, exploreSection]) {
			await within(section).findByText(OVERRIDE_MALFORMED_WARNING);
		}

		const generalSaveButton = within(generalSection).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(generalSaveButton).toBeEnabled();
		});
		await userEvent.click(generalSaveButton);
		await waitFor(() => {
			expect(args.onSaveGeneralModelOverride).toHaveBeenCalledWith(
				{ model_config_id: "" },
				expect.anything(),
			);
		});

		const exploreSaveButton = within(exploreSection).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(exploreSaveButton).toBeEnabled();
		});
		await userEvent.click(exploreSaveButton);
		await waitFor(() => {
			expect(args.onSaveExploreModelOverride).toHaveBeenCalledWith(
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
		exploreModelOverrideData: buildOverrideData("explore", {
			model_config_id: exploreDisabledModelConfig.id,
		}),
	}),
	play: async ({ canvasElement }) => {
		const generalSection = await getSection(canvasElement, "General model");
		const exploreSection = await getSection(
			canvasElement,
			"Explore subagent model",
		);

		for (const section of [generalSection, exploreSection]) {
			await within(section).findByText(UNAVAILABLE_SAVED_MODEL_WARNING);
			expect(
				within(section).getByRole("combobox", { name: "Unavailable model" }),
			).toBeInTheDocument();
		}
	},
};
