import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatModelConfig } from "#/testHelpers/chatModels";
import {
	CoderAgentsPageView,
	type CoderAgentsPageViewProps,
} from "./CoderAgentsPageView";

const OVERRIDE_MALFORMED_WARNING =
	"The saved override is malformed and is being treated as unset. Click Save to clear it.";
const UNAVAILABLE_SAVED_MODEL_WARNING =
	"The saved model is no longer enabled and will be ignored until you choose a new override.";
const TITLE_UNAVAILABLE_SAVED_MODEL_WARNING =
	"The selected model is currently unavailable. Title generation will be skipped until you choose another model or clear this setting.";

const buildModelConfig = (
	overrides: Partial<TypesGen.ChatModelConfig>,
): TypesGen.ChatModelConfig => ({
	...MockChatModelConfig,
	id: "model-default",
	model: "gpt-4.1-mini",
	display_name: "GPT 4.1 Mini",
	context_limit: 1_000_000,
	created_at: "2026-03-12T12:00:00.000Z",
	updated_at: "2026-03-12T12:00:00.000Z",
	...overrides,
});

const buildOverrideData = (
	context: TypesGen.ChatModelOverrideContext,
	overrides: Partial<TypesGen.ChatModelOverrideResponse> = {},
): TypesGen.ChatModelOverrideResponse => ({
	context,
	model_config_id: "",
	is_malformed: false,
	...overrides,
});

const buildTitleGenerationModelOverrideData = (
	overrides: Partial<TypesGen.ChatModelOverrideResponse> = {},
): TypesGen.ChatModelOverrideResponse =>
	buildOverrideData("title_generation", overrides);

const generalModelConfig = buildModelConfig({
	id: "model-general-gpt-4.1-mini",
	display_name: "GPT 4.1 Mini",
});

const claudeSonnetModelConfig = buildModelConfig({
	id: "model-claude-sonnet-4",
	ai_provider_id: "provider-anthropic",
	model: "claude-sonnet-4",
	display_name: "Claude Sonnet 4",
	context_limit: 200_000,
});

const advisorReasoningModelConfig = buildModelConfig({
	id: "model-advisor-gpt-5.2",
	model: "gpt-5.2",
	display_name: "GPT 5.2",
	context_limit: 200_000,
	model_config: {
		reasoning_effort: { default: "medium", max: "xhigh" },
	},
	reasoning_efforts: ["none", "minimal", "low", "medium", "high", "xhigh"],
});

const titleModelConfig = buildModelConfig({
	id: "model-title-gpt-4o-mini",
	model: "gpt-4o-mini",
	display_name: "GPT 4o Mini",
	context_limit: 128_000,
});

const exploreFallbackModelConfig = buildModelConfig({
	id: "model-explore-blank-display",
	ai_provider_id: "provider-anthropic",
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

const titleDisabledModelConfig = buildModelConfig({
	id: "model-title-disabled",
	model: "gpt-4o-mini-legacy",
	display_name: "GPT 4o Mini Legacy",
	enabled: false,
	context_limit: 128_000,
});

const exploreDisabledModelConfig = buildModelConfig({
	id: "model-explore-disabled",
	ai_provider_id: "provider-anthropic",
	model: "claude-haiku-legacy",
	display_name: "Claude Haiku Legacy",
	enabled: false,
	context_limit: 200_000,
});

const compactionDisabledModelConfig = buildModelConfig({
	id: "model-compaction-disabled",
	model: "gpt-4.1-nano-legacy",
	display_name: "GPT 4.1 Nano Legacy",
	enabled: false,
	context_limit: 128_000,
});

const providerDisabledModelConfig = buildModelConfig({
	id: "model-provider-disabled",
	ai_provider_id: "provider-openai-disabled",
	model: "gpt-4o-secondary",
	display_name: "GPT 4o Secondary",
	context_limit: 128_000,
});

const allModelConfigs: TypesGen.ChatModelConfig[] = [
	generalModelConfig,
	claudeSonnetModelConfig,
	advisorReasoningModelConfig,
	titleModelConfig,
	exploreFallbackModelConfig,
	generalDisabledModelConfig,
	titleDisabledModelConfig,
	exploreDisabledModelConfig,
	compactionDisabledModelConfig,
	providerDisabledModelConfig,
];

const providerInfoByID = new Map([
	[
		"provider-1",
		{ provider: "openai", displayName: "OpenAI", icon: "", enabled: true },
	],
	[
		"provider-anthropic",
		{
			provider: "anthropic",
			displayName: "Anthropic",
			icon: "",
			enabled: true,
		},
	],
	[
		"provider-openai-disabled",
		{
			provider: "openai",
			displayName: "OpenAI Secondary",
			icon: "",
			enabled: false,
		},
	],
]);

const buildArgs = (
	overrides: Partial<CoderAgentsPageViewProps> = {},
): CoderAgentsPageViewProps => ({
	adminOverridesData: { allow_users: false },
	adminOverridesError: undefined,
	onRetryAdminOverrides: fn(),
	isRetryingAdminOverrides: false,
	onSaveAdminOverrides: fn(),
	isSavingAdminOverrides: false,
	isSaveAdminOverridesError: false,
	generalModelOverrideData: buildOverrideData("general"),
	titleGenerationModelOverrideData: buildTitleGenerationModelOverrideData(),
	compactionModelOverrideData: buildOverrideData("compaction"),
	exploreModelOverrideData: buildOverrideData("explore"),
	modelConfigsData: allModelConfigs,
	providerInfoByID,
	modelConfigsError: undefined,
	isLoadingModelConfigs: false,
	isFetchingModelConfigs: false,
	onSaveGeneralModelOverride: fn(),
	isSavingGeneralModelOverride: false,
	isSaveGeneralModelOverrideError: false,
	onSaveTitleGenerationModel: fn(),
	isSavingTitleGenerationModel: false,
	isSaveTitleGenerationModelError: false,
	onSaveCompactionModel: fn(),
	isSavingCompactionModel: false,
	isSaveCompactionModelError: false,
	onSaveExploreModelOverride: fn(),
	isSavingExploreModelOverride: false,
	isSaveExploreModelOverrideError: false,
	showAdvisorSettings: false,
	advisorConfigData: undefined,
	isAdvisorConfigLoading: false,
	isAdvisorConfigFetching: false,
	isAdvisorConfigLoadError: false,
	onSaveAdvisorConfig: fn(),
	isSavingAdvisorConfig: false,
	isSaveAdvisorConfigError: false,
	saveAdvisorConfigError: undefined,
	showVirtualDesktopSettings: false,
	computerUseProviderData: undefined,
	isLoadingComputerUseProvider: false,
	onSaveComputerUseProvider: fn(),
	isSavingComputerUseProvider: false,
	computerUseProviderSaveError: null,
	...overrides,
});

const getSection = async (
	canvasElement: HTMLElement,
	headingName: string,
): Promise<HTMLElement> => {
	const canvas = within(canvasElement);
	const heading = await canvas.findByRole("heading", { name: headingName });
	const setting = heading.closest("form");
	if (!(setting instanceof HTMLElement)) {
		throw new Error(`Expected ${headingName} heading to live inside a form.`);
	}
	return setting;
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
	title: "pages/AISettingsPage/CoderAgentsPage/CoderAgentsPageView",
	component: CoderAgentsPageView,
	args: buildArgs(),
} satisfies Meta<typeof CoderAgentsPageView>;

export default meta;
type Story = StoryObj<typeof CoderAgentsPageView>;

export const AllOverridesUnset: Story = {
	args: buildArgs(),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByRole("heading", { name: "Coder Agents" }),
		).toBeVisible();
		expect(
			canvas.getByText(
				"Configure deployment-wide defaults for Coder Agents and agent-specific capabilities.",
			),
		).toBeVisible();

		expect(canvas.getByText("Allow personal model overrides")).toBeVisible();
		const headings = await canvas.findAllByRole("heading", { level: 3 });
		expect(headings.map((heading) => heading.textContent?.trim())).toEqual([
			"General model",
			"Title generation model",
			"Compaction model",
			"Explore subagent model",
		]);
		await canvas.findByText(
			"Leave unset to use Coder's title default, which prefers fast models from configured providers.",
		);

		const unsetSections = [
			{ headingName: "General model", placeholder: "Use chat default" },
			{
				headingName: "Title generation model",
				placeholder: "Use title default",
			},
			{
				headingName: "Compaction model",
				placeholder: "Use chat model",
			},
			{
				headingName: "Explore subagent model",
				placeholder: "Use chat default",
			},
		];
		for (const { headingName, placeholder } of unsetSections) {
			const section = await getSection(canvasElement, headingName);
			expect(
				within(section).getByRole("combobox", { name: placeholder }),
			).toBeInTheDocument();
			expect(
				within(section).queryByRole("button", { name: "Save" }),
			).not.toBeInTheDocument();
		}
	},
};

export const PersonalOverridesDisabled: Story = {
	args: buildArgs({
		adminOverridesData: { allow_users: false },
	}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow personal model overrides",
		});

		expect(toggle).not.toBeChecked();
	},
};

export const PersonalOverridesEnabled: Story = {
	args: buildArgs({
		adminOverridesData: { allow_users: true },
	}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow personal model overrides",
		});

		expect(toggle).toBeChecked();
	},
};

export const PersonalOverridesLoadError: Story = {
	args: buildArgs({
		adminOverridesData: undefined,
		adminOverridesError: new Error("Failed to load personal model overrides."),
	}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			await canvas.findByText("Failed to load personal model overrides."),
		).toBeInTheDocument();
		expect(
			canvas.queryByText("Loading personal model override settings..."),
		).not.toBeInTheDocument();
	},
};

export const EachOverrideSetToEnabledModel: Story = {
	args: buildArgs({
		generalModelOverrideData: buildOverrideData("general", {
			model_config_id: generalModelConfig.id,
		}),
		titleGenerationModelOverrideData: buildTitleGenerationModelOverrideData({
			model_config_id: titleModelConfig.id,
		}),
		compactionModelOverrideData: buildOverrideData("compaction", {
			model_config_id: claudeSonnetModelConfig.id,
		}),
		exploreModelOverrideData: buildOverrideData("explore", {
			model_config_id: exploreFallbackModelConfig.id,
		}),
	}),
	play: async ({ canvasElement, args }) => {
		const generalSection = await getSection(canvasElement, "General model");
		const titleSection = await getSection(
			canvasElement,
			"Title generation model",
		);
		const compactionSection = await getSection(
			canvasElement,
			"Compaction model",
		);
		const exploreSection = await getSection(
			canvasElement,
			"Explore subagent model",
		);

		expect(
			within(exploreSection).getByRole("combobox", {
				name: /claude-sonnet-4-20250514/i,
			}),
		).toHaveTextContent("claude-sonnet-4-20250514");

		expect(
			within(titleSection).getByRole("combobox", {
				name: /gpt 4o mini/i,
			}),
		).toHaveTextContent("GPT 4o Mini");

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

		await selectModelInSection(
			titleSection,
			canvasElement,
			/gpt 4o mini/i,
			"Claude Sonnet 4",
		);
		const titleSaveButton = within(titleSection).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(titleSaveButton).toBeEnabled();
		});
		await userEvent.click(titleSaveButton);
		await waitFor(() => {
			expect(args.onSaveTitleGenerationModel).toHaveBeenCalledWith(
				{ model_config_id: claudeSonnetModelConfig.id },
				expect.anything(),
			);
		});

		await selectModelInSection(
			compactionSection,
			canvasElement,
			/claude sonnet 4/i,
			"GPT 4o Mini",
		);
		const compactionSaveButton = within(compactionSection).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(compactionSaveButton).toBeEnabled();
		});
		await userEvent.click(compactionSaveButton);
		await waitFor(() => {
			expect(args.onSaveCompactionModel).toHaveBeenCalledWith(
				{ model_config_id: titleModelConfig.id },
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
	args: buildArgs({
		generalModelOverrideData: buildOverrideData("general", {
			is_malformed: true,
		}),
		titleGenerationModelOverrideData: buildTitleGenerationModelOverrideData({
			is_malformed: true,
		}),
		compactionModelOverrideData: buildOverrideData("compaction", {
			is_malformed: true,
		}),
		exploreModelOverrideData: buildOverrideData("explore", {
			is_malformed: true,
		}),
	}),
	play: async ({ canvasElement, args }) => {
		const generalSection = await getSection(canvasElement, "General model");
		const titleSection = await getSection(
			canvasElement,
			"Title generation model",
		);
		const compactionSection = await getSection(
			canvasElement,
			"Compaction model",
		);
		const exploreSection = await getSection(
			canvasElement,
			"Explore subagent model",
		);

		for (const section of [
			generalSection,
			titleSection,
			compactionSection,
			exploreSection,
		]) {
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

		const titleSaveButton = within(titleSection).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(titleSaveButton).toBeEnabled();
		});
		await userEvent.click(titleSaveButton);
		await waitFor(() => {
			expect(args.onSaveTitleGenerationModel).toHaveBeenCalledWith(
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

		const compactionSaveButton = within(compactionSection).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(compactionSaveButton).toBeEnabled();
		});
		await userEvent.click(compactionSaveButton);
		await waitFor(() => {
			expect(args.onSaveCompactionModel).toHaveBeenCalledWith(
				{ model_config_id: "" },
				expect.anything(),
			);
		});
	},
};

export const UnavailableSavedModels: Story = {
	args: buildArgs({
		generalModelOverrideData: buildOverrideData("general", {
			model_config_id: generalDisabledModelConfig.id,
		}),
		titleGenerationModelOverrideData: buildTitleGenerationModelOverrideData({
			model_config_id: titleDisabledModelConfig.id,
		}),
		compactionModelOverrideData: buildOverrideData("compaction", {
			model_config_id: compactionDisabledModelConfig.id,
		}),
		exploreModelOverrideData: buildOverrideData("explore", {
			model_config_id: exploreDisabledModelConfig.id,
		}),
	}),
	play: async ({ canvasElement }) => {
		const generalSection = await getSection(canvasElement, "General model");
		const titleSection = await getSection(
			canvasElement,
			"Title generation model",
		);
		const compactionSection = await getSection(
			canvasElement,
			"Compaction model",
		);
		const exploreSection = await getSection(
			canvasElement,
			"Explore subagent model",
		);

		for (const section of [generalSection, compactionSection, exploreSection]) {
			await within(section).findByText(UNAVAILABLE_SAVED_MODEL_WARNING);
			expect(
				within(section).getByRole("combobox", { name: "Unavailable model" }),
			).toBeInTheDocument();
		}
		await within(titleSection).findByText(
			TITLE_UNAVAILABLE_SAVED_MODEL_WARNING,
		);
		expect(
			within(titleSection).getByRole("combobox", {
				name: "Unavailable model",
			}),
		).toBeInTheDocument();
	},
};

export const AdvisorSettingsVisible: Story = {
	args: buildArgs({
		showAdvisorSettings: true,
		advisorConfigData: {
			enabled: true,
			max_uses_per_run: 3,
			max_output_tokens: 16384,
			model_config_id: "00000000-0000-0000-0000-000000000000",
		},
	}),
	play: async ({ canvasElement, args }) => {
		const section = await getSection(canvasElement, "Advisor");
		expect(
			within(section).getByRole("spinbutton", {
				name: "Max uses per turn",
			}),
		).toHaveValue(3);
		expect(
			within(section).getByRole("spinbutton", {
				name: "Max output tokens",
			}),
		).toHaveValue(16384);
		expect(
			within(section).getByRole("combobox", { name: "Use chat model" }),
		).toBeInTheDocument();

		// Changing a value exposes the Save button.
		const maxUses = within(section).getByRole("spinbutton", {
			name: "Max uses per turn",
		});
		await userEvent.clear(maxUses);
		await userEvent.type(maxUses, "5");
		const saveButton = within(section).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);
		await waitFor(() => {
			expect(args.onSaveAdvisorConfig).toHaveBeenCalledWith(
				expect.objectContaining({ max_uses_per_run: 5 }),
				expect.anything(),
			);
		});
	},
};

export const AdvisorReasoningEffort: Story = {
	args: buildArgs({
		showAdvisorSettings: true,
		advisorConfigData: {
			enabled: true,
			max_uses_per_run: 3,
			max_output_tokens: 16384,
			model_config_id: advisorReasoningModelConfig.id,
			reasoning_effort: "medium",
		},
	}),
	play: async ({ canvasElement, args }) => {
		const section = await getSection(canvasElement, "Advisor");
		const modelSelector = within(section).getByRole("combobox", {
			name: "GPT 5.2",
		});
		expect(modelSelector).toBeInTheDocument();
		const maxUses = within(section).getByRole("spinbutton", {
			name: "Max uses per turn",
		});
		await userEvent.clear(maxUses);
		await userEvent.type(maxUses, "5");

		const saveButton = await within(section).findByRole("button", {
			name: "Save",
		});
		expect(saveButton).toBeEnabled();
		await userEvent.click(saveButton);
		await waitFor(() => {
			expect(args.onSaveAdvisorConfig).toHaveBeenCalledWith(
				expect.objectContaining({
					max_uses_per_run: 5,
					model_config_id: advisorReasoningModelConfig.id,
					reasoning_effort: "medium",
				}),
				expect.anything(),
			);
		});
	},
};

export const DisabledProviderModelsHidden: Story = {
	args: buildArgs({
		showAdvisorSettings: true,
		advisorConfigData: {
			enabled: true,
			max_uses_per_run: 3,
			max_output_tokens: 16384,
			model_config_id: "00000000-0000-0000-0000-000000000000",
		},
	}),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		const generalSection = await getSection(canvasElement, "General model");
		const generalTrigger = within(generalSection).getByRole("combobox", {
			name: "Use chat default",
		});
		await userEvent.click(generalTrigger);
		expect(
			await body.findByRole("option", { name: /GPT 4\.1 Mini/ }),
		).toBeInTheDocument();
		expect(
			body.queryByRole("option", { name: /GPT 4o Secondary/ }),
		).not.toBeInTheDocument();
		await userEvent.keyboard("{Escape}");

		const advisorSection = await getSection(canvasElement, "Advisor");
		const advisorTrigger = within(advisorSection).getByRole("combobox", {
			name: "Use chat model",
		});
		await userEvent.click(advisorTrigger);
		expect(
			await body.findByRole("option", { name: /GPT 4\.1 Mini/ }),
		).toBeInTheDocument();
		expect(
			body.queryByRole("option", { name: /GPT 4o Secondary/ }),
		).not.toBeInTheDocument();
		await userEvent.keyboard("{Escape}");
	},
};

export const AdvisorClearButton: Story = {
	args: buildArgs({
		showAdvisorSettings: true,
		advisorConfigData: {
			enabled: true,
			max_uses_per_run: 3,
			max_output_tokens: 16384,
			model_config_id: advisorReasoningModelConfig.id,
			reasoning_effort: "high",
		},
	}),
	play: async ({ canvasElement, args }) => {
		const section = await getSection(canvasElement, "Advisor");
		const clearButton = within(section).getByRole("button", { name: "Clear" });
		await userEvent.click(clearButton);
		expect(
			within(section).getByRole("spinbutton", {
				name: "Max uses per turn",
			}),
		).toHaveValue(0);
		expect(
			within(section).getByRole("spinbutton", {
				name: "Max output tokens",
			}),
		).toHaveValue(0);
		expect(
			within(section).getByRole("combobox", { name: "Use chat model" }),
		).toHaveTextContent("Use chat model");
		const saveButton = within(section).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);
		await waitFor(() => {
			expect(args.onSaveAdvisorConfig).toHaveBeenCalledWith(
				{
					enabled: true,
					max_uses_per_run: 0,
					max_output_tokens: 0,
					model_config_id: "00000000-0000-0000-0000-000000000000",
				},
				expect.anything(),
			);
		});
	},
};

export const VirtualDesktopSettingsVisible: Story = {
	args: buildArgs({
		showVirtualDesktopSettings: true,
		computerUseProviderData: { provider: "anthropic" },
	}),
	play: async ({ canvasElement }) => {
		const section = await getSection(canvasElement, "Virtual desktop");
		expect(
			within(section).getByRole("combobox", {
				name: "Computer use provider",
			}),
		).toHaveTextContent("Anthropic");
	},
};

export const VirtualDesktopProviderChange: Story = {
	args: buildArgs({
		showVirtualDesktopSettings: true,
		computerUseProviderData: { provider: "anthropic" },
	}),
	play: async ({ canvasElement, args }) => {
		const section = await getSection(canvasElement, "Virtual desktop");
		const trigger = within(section).getByRole("combobox", {
			name: "Computer use provider",
		});
		await userEvent.click(trigger);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(await body.findByRole("option", { name: "OpenAI" }));
		const saveButton = within(section).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);
		await waitFor(() => {
			expect(args.onSaveComputerUseProvider).toHaveBeenCalledWith(
				{ provider: "openai" },
				expect.anything(),
			);
		});
	},
};
