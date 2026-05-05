import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
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
	provider: "anthropic",
	model: "claude-sonnet-4",
	display_name: "Claude Sonnet 4",
	context_limit: 200_000,
});

const disabledModelConfig = buildModelConfig({
	id: "model-disabled",
	model: "gpt-4.1-legacy",
	display_name: "GPT 4.1 Legacy",
	enabled: false,
});

const inaccessibleModelConfig = buildModelConfig({
	id: "model-inaccessible",
	provider: "bedrock",
	model: "claude-3-5-sonnet",
	display_name: "Bedrock Claude",
});

const modelConfigs = [
	defaultModelConfig,
	claudeModelConfig,
	disabledModelConfig,
	inaccessibleModelConfig,
];

const modelOptions: ModelSelectorOption[] = [
	{
		id: defaultModelConfig.id,
		provider: defaultModelConfig.provider,
		model: defaultModelConfig.model,
		displayName: defaultModelConfig.display_name,
		contextLimit: defaultModelConfig.context_limit,
	},
	{
		id: claudeModelConfig.id,
		provider: claudeModelConfig.provider,
		model: claudeModelConfig.model,
		displayName: claudeModelConfig.display_name,
		contextLimit: claudeModelConfig.context_limit,
	},
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

const makeArgs = (
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
	comboboxName: string | RegExp,
	optionName: string | RegExp,
) => {
	await userEvent.click(
		within(section).getByRole("combobox", { name: comboboxName }),
	);
	const body = within(canvasElement.ownerDocument.body);
	await userEvent.click(await body.findByRole("option", { name: optionName }));
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsUserAgentsPageView",
	component: AgentSettingsUserAgentsPageView,
	args: makeArgs(),
} satisfies Meta<typeof AgentSettingsUserAgentsPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsUserAgentsPageView>;

export const EnabledWithNoSavedValues: Story = {
	args: makeArgs(),
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
	args: makeArgs({
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
		await selectOption(
			rootSection,
			canvasElement,
			"Root agent model behavior",
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
			"General subagent model behavior",
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

export const MalformedSavedValues: Story = {
	args: makeArgs({
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
	args: makeArgs({
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
	args: makeArgs({
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
	args: makeArgs({
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
			"Root agent model behavior",
			/Chat default/i,
		);
		await selectOption(
			generalSection,
			canvasElement,
			"General subagent model behavior",
			/Deployment default/i,
		);
		await selectOption(
			exploreSection,
			canvasElement,
			"Explore subagent model behavior",
			/Chat default/i,
		);

		expect(rootSection).toHaveTextContent("Chat default");
		expect(generalSection).toHaveTextContent("Deployment default");
		expect(exploreSection).toHaveTextContent("Chat default");
	},
};

export const LoadingState: Story = {
	args: makeArgs({
		overridesData: undefined,
		isLoadingOverrides: true,
		modelOptions: [],
		isLoadingModels: true,
	}),
	play: async ({ canvasElement }) => {
		const rootSection = await getSection(canvasElement, "Root agent model");
		expect(
			within(rootSection).getByRole("combobox", {
				name: "Root agent model behavior",
			}),
		).toBeDisabled();
		expect(
			within(rootSection).getByRole("button", { name: "Save" }),
		).toBeDisabled();
	},
};

export const OverridesError: Story = {
	args: makeArgs({
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
	args: makeArgs({
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
	args: makeArgs({
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
				name: "Root agent model behavior",
			}),
		).toBeDisabled();
		expect(
			within(rootSection).getByRole("button", { name: "Save" }),
		).toBeDisabled();
	},
};

export const InvalidRootDeploymentDefault: Story = {
	args: makeArgs({
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
			"Root agent model behavior",
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
