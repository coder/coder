import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatModelConfig } from "#/testHelpers/chatModels";
import {
	AgentSettingsUserAgentsPageView,
	type AgentSettingsUserAgentsPageViewProps,
} from "./AgentSettingsUserAgentsPageView";
import type { ModelSelectorOption } from "./components/ChatElements";

const MALFORMED_WARNING =
	"The saved override is malformed. Choose a valid value and save to replace it.";
const UNAVAILABLE_WARNING =
	"The saved model is unavailable and will be ignored until you choose a valid model override.";

const buildModelConfig = (
	overrides: Partial<TypesGen.ChatModelConfig> = {},
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

const buildOverride = (
	context: TypesGen.ChatPersonalModelOverrideContext,
	overrides: Partial<TypesGen.ChatPersonalModelOverride> = {},
): TypesGen.ChatPersonalModelOverride => ({
	context,
	mode: context === "root" ? "chat_default" : "deployment_default",
	model_config_id: "",
	is_set: false,
	is_malformed: false,
	...overrides,
});

const buildDeploymentDefault = (
	context: TypesGen.ChatModelOverrideContext,
	overrides: Partial<TypesGen.ChatModelOverrideResponse> = {},
): TypesGen.ChatModelOverrideResponse => ({
	context,
	model_config_id: "",
	is_malformed: false,
	...overrides,
});

const buildDeploymentDefaults = (
	overrides: Partial<TypesGen.ChatPersonalModelOverrideDeploymentDefaults> = {},
): TypesGen.ChatPersonalModelOverrideDeploymentDefaults => ({
	general: buildDeploymentDefault("general"),
	explore: buildDeploymentDefault("explore"),
	...overrides,
});

const defaultModelConfig = buildModelConfig({
	id: "model-gpt-4.1-mini",
	display_name: "GPT 4.1 Mini",
	is_default: true,
});

const claudeModelConfig = buildModelConfig({
	id: "model-claude-sonnet-4",
	ai_provider_id: "provider-anthropic",
	model: "claude-sonnet-4",
	display_name: "Claude Sonnet 4",
	context_limit: 200_000,
});

const reasoningModelConfig = buildModelConfig({
	id: "model-gpt-5",
	model: "gpt-5",
	display_name: "GPT-5",
	model_config: {
		reasoning_effort: { default: "medium", max: "high" },
	},
	reasoning_efforts: ["none", "minimal", "low", "medium", "high"],
});

const disabledModelConfig = buildModelConfig({
	id: "model-disabled",
	model: "gpt-4.1-legacy",
	display_name: "GPT 4.1 Legacy",
	enabled: false,
});

const inaccessibleModelConfig = buildModelConfig({
	id: "model-inaccessible",
	ai_provider_id: "provider-bedrock",
	model: "claude-3-5-sonnet",
	display_name: "Bedrock Claude",
});

const modelConfigs = [
	defaultModelConfig,
	claudeModelConfig,
	reasoningModelConfig,
	disabledModelConfig,
	inaccessibleModelConfig,
];

const reasoningModelOption: ModelSelectorOption = {
	id: reasoningModelConfig.id,
	provider: "openai",
	model: reasoningModelConfig.model,
	displayName: reasoningModelConfig.display_name,
	contextLimit: reasoningModelConfig.context_limit,
	reasoningEffortDefault: "medium",
	reasoningEfforts: ["none", "minimal", "low", "medium", "high"],
};

const modelOptions: ModelSelectorOption[] = [
	{
		id: defaultModelConfig.id,
		provider: "openai",
		model: defaultModelConfig.model,
		displayName: defaultModelConfig.display_name,
		contextLimit: defaultModelConfig.context_limit,
	},
	{
		id: claudeModelConfig.id,
		provider: "anthropic",
		model: claudeModelConfig.model,
		displayName: claudeModelConfig.display_name,
		contextLimit: claudeModelConfig.context_limit,
	},
	reasoningModelOption,
];

const buildOverridesResponse = (
	overrides: Partial<TypesGen.UserChatPersonalModelOverridesResponse> = {},
): TypesGen.UserChatPersonalModelOverridesResponse => ({
	enabled: true,
	root: buildOverride("root"),
	general: buildOverride("general"),
	explore: buildOverride("explore"),
	deployment_defaults: buildDeploymentDefaults({
		general: buildDeploymentDefault("general", {
			model_config_id: claudeModelConfig.id,
		}),
		explore: buildDeploymentDefault("explore", {
			model_config_id: claudeModelConfig.id,
		}),
	}),
	...overrides,
});

const buildArgs = (
	overrides: Partial<AgentSettingsUserAgentsPageViewProps> = {},
): AgentSettingsUserAgentsPageViewProps => ({
	overridesData: buildOverridesResponse(),
	overridesError: undefined,
	onRetryOverrides: fn(),
	isRetryingOverrides: false,
	isLoadingOverrides: false,
	modelOptions,
	modelConfigs,
	modelConfigsError: undefined,
	isLoadingModels: false,
	onSaveRootModelOverride: fn(),
	isSavingRootModelOverride: false,
	isSaveRootModelOverrideError: false,
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

const selectOption = async (
	section: HTMLElement,
	canvasElement: HTMLElement,
	comboboxName: string,
	optionName: string | RegExp,
) => {
	const combobox = within(section).getByRole("combobox", {
		name: comboboxName,
	});
	await userEvent.click(combobox);
	const body = within(canvasElement.ownerDocument.body);
	await userEvent.click(await body.findByRole("option", { name: optionName }));
	return combobox;
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsUserAgentsPageView",
	component: AgentSettingsUserAgentsPageView,
	args: buildArgs(),
} satisfies Meta<typeof AgentSettingsUserAgentsPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsUserAgentsPageView>;

export const EnabledWithNoSavedValues: Story = {
	args: buildArgs(),
	play: async ({ canvasElement }) => {
		const rootSection = await getSection(canvasElement, "Root agent model");
		const generalSection = await getSection(
			canvasElement,
			"General subagent model",
		);
		const exploreSection = await getSection(
			canvasElement,
			"Explore subagent model",
		);

		expect(rootSection).toHaveTextContent("Chat default: GPT 4.1 Mini");
		expect(generalSection).toHaveTextContent(
			"Deployment default: Claude Sonnet 4",
		);
		expect(exploreSection).toHaveTextContent(
			"Deployment default: Claude Sonnet 4",
		);

		for (const section of [rootSection, generalSection, exploreSection]) {
			expect(
				within(section).getByRole("button", { name: "Save" }),
			).toBeDisabled();
		}
	},
};

export const EnabledWithSavedValues: Story = {
	args: buildArgs({
		overridesData: buildOverridesResponse({
			root: buildOverride("root", {
				mode: "chat_default",
				is_set: true,
			}),
			general: buildOverride("general", {
				mode: "deployment_default",
				is_set: true,
			}),
			explore: buildOverride("explore", {
				mode: "model",
				model_config_id: claudeModelConfig.id,
				is_set: true,
			}),
		}),
	}),
	play: async ({ canvasElement, args }) => {
		const rootSection = await getSection(canvasElement, "Root agent model");
		const exploreSection = await getSection(
			canvasElement,
			"Explore subagent model",
		);
		expect(
			within(exploreSection).getByRole("combobox", {
				name: "Explore subagent model behavior, Claude Sonnet 4",
			}),
		).toHaveTextContent("Claude Sonnet 4");

		await selectOption(
			rootSection,
			canvasElement,
			"Root agent model behavior, Chat default: GPT 4.1 Mini",
			/Claude Sonnet 4/i,
		);
		const rootSaveButton = within(rootSection).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(rootSaveButton).toBeEnabled();
		});
		await userEvent.click(rootSaveButton);
		await waitFor(() => {
			expect(args.onSaveRootModelOverride).toHaveBeenCalledWith(
				{ mode: "model", model_config_id: claudeModelConfig.id },
				expect.anything(),
			);
		});

		const generalSection = await getSection(
			canvasElement,
			"General subagent model",
		);
		await selectOption(
			generalSection,
			canvasElement,
			"General subagent model behavior, Deployment default: Claude Sonnet 4",
			/Chat default/i,
		);
		await userEvent.click(
			within(generalSection).getByRole("button", { name: "Save" }),
		);

		await waitFor(() => {
			expect(args.onSaveGeneralModelOverride).toHaveBeenCalledWith(
				{ mode: "chat_default", model_config_id: "" },
				expect.anything(),
			);
		});
	},
};

export const SavedReasoningModel: Story = {
	args: buildArgs({
		modelOptions: [
			{
				id: defaultModelConfig.id,
				provider: "openai",
				model: defaultModelConfig.model,
				displayName: defaultModelConfig.display_name,
				contextLimit: defaultModelConfig.context_limit,
			},
			{
				id: reasoningModelConfig.id,
				provider: "openai",
				model: reasoningModelConfig.model,
				displayName: reasoningModelConfig.display_name,
				contextLimit: reasoningModelConfig.context_limit,
				reasoningEffortDefault: "medium",
				reasoningEfforts: ["none", "minimal", "low", "medium", "high"],
			},
		],
		overridesData: buildOverridesResponse({
			root: buildOverride("root", {
				mode: "model",
				model_config_id: defaultModelConfig.id,
				is_set: true,
			}),
		}),
	}),
	play: async ({ canvasElement, args }) => {
		const rootSection = await getSection(canvasElement, "Root agent model");
		const modelPicker = await selectOption(
			rootSection,
			canvasElement,
			"Root agent model behavior, GPT 4.1 Mini",
			/GPT-5/i,
		);

		const body = within(canvasElement.ownerDocument.body);
		expect(modelPicker).toHaveAttribute("aria-expanded", "true");
		expect(await body.findByRole("listbox")).toBeVisible();
		const slider = await body.findByRole("slider");
		expect(slider).toBeVisible();
		expect(slider).toHaveAttribute("aria-valuenow", "3");
		expect(body.getByText("Medium")).toBeVisible();

		const infoTrigger = body.getByRole("button", {
			name: "About reasoning effort",
		});
		await userEvent.tab();
		expect(infoTrigger).toHaveFocus();

		await userEvent.tab();
		expect(slider).toHaveFocus();
		await userEvent.keyboard("{ArrowRight}");
		await waitFor(() => {
			expect(slider).toHaveAttribute("aria-valuenow", "4");
		});
		expect(body.getByText("High")).toBeVisible();

		await userEvent.keyboard("{Escape}");
		await userEvent.click(
			within(rootSection).getByRole("button", { name: "Save" }),
		);
		await waitFor(() => {
			expect(args.onSaveRootModelOverride).toHaveBeenCalledWith(
				{
					mode: "model",
					model_config_id: reasoningModelConfig.id,
					reasoning_effort: "high",
				},
				expect.anything(),
			);
		});
	},
};

export const SavedLowReasoningEffort: Story = {
	args: buildArgs({
		modelOptions: [reasoningModelOption],
		overridesData: buildOverridesResponse({
			root: buildOverride("root", {
				mode: "model",
				model_config_id: reasoningModelConfig.id,
				reasoning_effort: "low",
				is_set: true,
			}),
		}),
	}),
	play: async ({ canvasElement }) => {
		const rootSection = await getSection(canvasElement, "Root agent model");
		const modelPicker = within(rootSection).getByRole("combobox", {
			name: "Root agent model behavior, GPT-5",
		});
		expect(modelPicker).toHaveTextContent("GPT-5");
		await userEvent.click(modelPicker);

		const body = within(canvasElement.ownerDocument.body);
		expect(modelPicker).toHaveAttribute("aria-expanded", "true");
		const slider = await body.findByRole("slider");
		expect(slider).toHaveAttribute("aria-valuenow", "2");
		await waitFor(() => {
			expect(body.getByText("Low")).toBeVisible();
		});
	},
};

export const MalformedSavedValues: Story = {
	args: buildArgs({
		overridesData: buildOverridesResponse({
			root: buildOverride("root", { is_malformed: true }),
			general: buildOverride("general", { is_malformed: true }),
			explore: buildOverride("explore", { is_malformed: true }),
		}),
	}),
	play: async ({ canvasElement, args }) => {
		const rootSection = await getSection(canvasElement, "Root agent model");
		const generalSection = await getSection(
			canvasElement,
			"General subagent model",
		);
		const exploreSection = await getSection(
			canvasElement,
			"Explore subagent model",
		);

		for (const section of [rootSection, generalSection, exploreSection]) {
			expect(within(section).getByText(MALFORMED_WARNING)).toBeInTheDocument();
			expect(
				within(section).getByRole("button", { name: "Save" }),
			).toBeEnabled();
		}

		await userEvent.click(
			within(rootSection).getByRole("button", { name: "Save" }),
		);
		await waitFor(() => {
			expect(args.onSaveRootModelOverride).toHaveBeenCalledWith(
				{ mode: "chat_default", model_config_id: "" },
				expect.anything(),
			);
		});
	},
};

export const MalformedEmptyModelSavedValues: Story = {
	args: buildArgs({
		overridesData: buildOverridesResponse({
			root: buildOverride("root", {
				mode: "model",
				model_config_id: "",
				is_set: true,
				is_malformed: true,
			}),
			general: buildOverride("general", {
				mode: "model",
				model_config_id: "",
				is_set: true,
				is_malformed: true,
			}),
			explore: buildOverride("explore", {
				mode: "model",
				model_config_id: "",
				is_set: true,
				is_malformed: true,
			}),
		}),
	}),
	play: async ({ canvasElement, args }) => {
		const rootSection = await getSection(canvasElement, "Root agent model");
		const generalSection = await getSection(
			canvasElement,
			"General subagent model",
		);
		const exploreSection = await getSection(
			canvasElement,
			"Explore subagent model",
		);

		expect(rootSection).toHaveTextContent("Chat default");
		expect(generalSection).toHaveTextContent("Deployment default");
		expect(exploreSection).toHaveTextContent("Deployment default");

		for (const section of [rootSection, generalSection, exploreSection]) {
			expect(within(section).getByText(MALFORMED_WARNING)).toBeInTheDocument();
			expect(
				within(section).getByRole("button", { name: "Save" }),
			).toBeEnabled();
		}

		await userEvent.click(
			within(rootSection).getByRole("button", { name: "Save" }),
		);
		await waitFor(() => {
			expect(args.onSaveRootModelOverride).toHaveBeenCalledWith(
				{ mode: "chat_default", model_config_id: "" },
				expect.anything(),
			);
		});

		await userEvent.click(
			within(generalSection).getByRole("button", { name: "Save" }),
		);
		await waitFor(() => {
			expect(args.onSaveGeneralModelOverride).toHaveBeenCalledWith(
				{ mode: "deployment_default", model_config_id: "" },
				expect.anything(),
			);
		});
	},
};

export const UnavailableSavedModels: Story = {
	args: buildArgs({
		overridesData: buildOverridesResponse({
			root: buildOverride("root", {
				mode: "model",
				model_config_id: disabledModelConfig.id,
				is_set: true,
			}),
			general: buildOverride("general", {
				mode: "model",
				model_config_id: inaccessibleModelConfig.id,
				is_set: true,
			}),
		}),
	}),
	play: async ({ canvasElement }) => {
		const rootSection = await getSection(canvasElement, "Root agent model");
		const generalSection = await getSection(
			canvasElement,
			"General subagent model",
		);

		expect(rootSection).toHaveTextContent("Unavailable: GPT 4.1 Legacy");
		expect(generalSection).toHaveTextContent("Unavailable: Bedrock Claude");
		expect(
			within(rootSection).getByText(UNAVAILABLE_WARNING),
		).toBeInTheDocument();
		expect(
			within(generalSection).getByText(UNAVAILABLE_WARNING),
		).toBeInTheDocument();
	},
};

export const ModelConfigsError: Story = {
	args: buildArgs({
		modelConfigsError: new Error("Failed to load model configs."),
		overridesData: buildOverridesResponse({
			root: buildOverride("root", {
				mode: "model",
				model_config_id: claudeModelConfig.id,
				is_set: true,
			}),
			general: buildOverride("general", {
				mode: "model",
				model_config_id: claudeModelConfig.id,
				is_set: true,
			}),
			explore: buildOverride("explore", {
				mode: "model",
				model_config_id: claudeModelConfig.id,
				is_set: true,
			}),
		}),
	}),
	play: async ({ canvasElement }) => {
		const rootSection = await getSection(canvasElement, "Root agent model");
		const generalSection = await getSection(
			canvasElement,
			"General subagent model",
		);
		const exploreSection = await getSection(
			canvasElement,
			"Explore subagent model",
		);

		for (const section of [rootSection, generalSection, exploreSection]) {
			expect(
				within(section).getByText("Failed to load model configs."),
			).toBeInTheDocument();
			expect(
				within(section).getByRole("combobox", { name: /behavior/i }),
			).toBeEnabled();
		}

		await selectOption(
			rootSection,
			canvasElement,
			"Root agent model behavior, Claude Sonnet 4",
			/Chat default/i,
		);
		await selectOption(
			generalSection,
			canvasElement,
			"General subagent model behavior, Claude Sonnet 4",
			/Deployment default/i,
		);
		await selectOption(
			exploreSection,
			canvasElement,
			"Explore subagent model behavior, Claude Sonnet 4",
			/Chat default/i,
		);

		expect(rootSection).toHaveTextContent("Chat default");
		expect(generalSection).toHaveTextContent("Deployment default");
		expect(exploreSection).toHaveTextContent("Chat default");
	},
};

export const LoadingState: Story = {
	args: buildArgs({
		overridesData: undefined,
		isLoadingOverrides: true,
		modelOptions: [],
		isLoadingModels: true,
	}),
	play: async ({ canvasElement }) => {
		const rootSection = await getSection(canvasElement, "Root agent model");
		expect(
			within(rootSection).getByRole("combobox", {
				name: "Root agent model behavior, Chat default: GPT 4.1 Mini",
			}),
		).toBeDisabled();
		expect(
			within(rootSection).getByRole("button", { name: "Save" }),
		).toBeDisabled();
	},
};

export const OverridesError: Story = {
	args: buildArgs({
		overridesData: undefined,
		overridesError: new Error("Failed to load overrides"),
	}),
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Failed to load overrides"),
		).toBeInTheDocument();

		const retryButton = canvas.getByRole("button", { name: "Retry" });
		expect(retryButton).toBeEnabled();
		await userEvent.click(retryButton);
		expect(args.onRetryOverrides).toHaveBeenCalled();

		const rootSection = await getSection(canvasElement, "Root agent model");
		const generalSection = await getSection(
			canvasElement,
			"General subagent model",
		);
		const exploreSection = await getSection(
			canvasElement,
			"Explore subagent model",
		);
		for (const section of [rootSection, generalSection, exploreSection]) {
			expect(
				within(section).getByRole("combobox", { name: /behavior/i }),
			).toBeDisabled();
			expect(
				within(section).getByRole("button", { name: "Save" }),
			).toBeDisabled();
		}
	},
};

export const SaveErrorState: Story = {
	args: buildArgs({
		isSaveGeneralModelOverrideError: true,
	}),
	play: async ({ canvasElement }) => {
		const generalSection = await getSection(
			canvasElement,
			"General subagent model",
		);
		expect(
			within(generalSection).getByText(
				"Failed to save general subagent model override.",
			),
		).toBeInTheDocument();
	},
};

export const AdminDisabledReadOnly: Story = {
	args: buildArgs({
		overridesData: buildOverridesResponse({
			enabled: false,
			root: buildOverride("root", {
				mode: "model",
				model_config_id: defaultModelConfig.id,
				is_set: true,
			}),
		}),
	}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText(
				/Personal model overrides are disabled by an administrator/i,
			),
		).toBeInTheDocument();
		const rootSection = await getSection(canvasElement, "Root agent model");
		expect(
			within(rootSection).getByRole("combobox", {
				name: "Root agent model behavior, GPT 4.1 Mini",
			}),
		).toBeDisabled();
		expect(
			within(rootSection).getByRole("button", { name: "Save" }),
		).toBeDisabled();
	},
};

export const InvalidRootDeploymentDefault: Story = {
	args: buildArgs({
		overridesData: buildOverridesResponse({
			root: buildOverride("root", {
				mode: "deployment_default",
				is_set: true,
			}),
		}),
	}),
	play: async ({ canvasElement, args }) => {
		const rootSection = await getSection(canvasElement, "Root agent model");
		expect(rootSection).toHaveTextContent("Invalid deployment default");
		expect(
			within(rootSection).getByText(
				/The saved root override uses the deployment default/i,
			),
		).toBeInTheDocument();
		expect(
			within(rootSection).getByRole("button", { name: "Save" }),
		).toBeDisabled();

		await selectOption(
			rootSection,
			canvasElement,
			"Root agent model behavior, Invalid deployment default",
			/Chat default/i,
		);
		await userEvent.click(
			within(rootSection).getByRole("button", { name: "Save" }),
		);
		await waitFor(() => {
			expect(args.onSaveRootModelOverride).toHaveBeenCalledWith(
				{ mode: "chat_default", model_config_id: "" },
				expect.anything(),
			);
		});
	},
};
