import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, screen, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import { MockChatModel } from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import {
	AgentSettingsUserAgentsPageView,
	type AgentSettingsUserAgentsPageViewProps,
} from "./AgentSettingsUserAgentsPageView";

const buildModelConfig = (
	overrides: Partial<TypesGen.ChatModel> = {},
): TypesGen.ChatModel => ({
	...MockChatModel,
	id: "model-default",
	model: "gpt-4.1-mini",
	display_name: "GPT 4.1 Mini",
	context_limit: 1_000_000,
	created_at: "2026-03-12T12:00:00.000Z",
	updated_at: "2026-03-12T12:00:00.000Z",
	...overrides,
});

export const buildOverride = (
	context: TypesGen.ChatPersonalModelOverrideContext,
	overrides: Partial<TypesGen.ChatPersonalModelOverride> = {},
): TypesGen.ChatPersonalModelOverride => ({
	context,
	mode: context === "root" ? "chat_default" : "deployment_default",
	model_config_id: "",
	is_set: false,
	...overrides,
});

const buildDeploymentDefault = (
	context: TypesGen.ChatModelOverrideContext,
	overrides: Partial<TypesGen.ChatModelOverrideResponse> = {},
): TypesGen.ChatModelOverrideResponse => ({
	context,
	model_config_id: "",
	...overrides,
});

const buildDeploymentDefaults = (
	overrides: Partial<TypesGen.ChatPersonalModelOverrideDeploymentDefaults> = {},
): TypesGen.ChatPersonalModelOverrideDeploymentDefaults => ({
	general: buildDeploymentDefault("general"),
	explore: buildDeploymentDefault("explore"),
	...overrides,
});

export const defaultModelConfig = buildModelConfig({
	id: "model-gpt-4.1-mini",
	display_name: "GPT 4.1 Mini",
	is_default: true,
});

export const claudeModelConfig = buildModelConfig({
	id: "model-claude-sonnet-4",
	ai_provider_id: "provider-anthropic",
	model: "claude-sonnet-4",
	display_name: "Claude Sonnet 4",
	context_limit: 200_000,
});

export const reasoningModelConfig = buildModelConfig({
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

const models = [
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

const organization2ModelConfig = buildModelConfig({
	id: "organization-2-model",
	organization_id: MockOrganization2.id,
	model: "organization-two-model",
	display_name: "Organization Two Model",
	is_default: true,
});

const organization2ModelOption: ModelSelectorOption = {
	id: organization2ModelConfig.id,
	provider: "openai",
	model: organization2ModelConfig.model,
	displayName: organization2ModelConfig.display_name,
	contextLimit: organization2ModelConfig.context_limit,
};

export const modelOptions: ModelSelectorOption[] = [
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

export const buildOverridesResponse = (
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

export const buildArgs = (
	overrides: Partial<AgentSettingsUserAgentsPageViewProps> = {},
): AgentSettingsUserAgentsPageViewProps => ({
	overridesData: buildOverridesResponse(),
	overridesError: undefined,
	onRetryOverrides: fn(),
	isRetryingOverrides: false,
	isLoading: false,
	modelOptions,
	models,
	modelsError: undefined,
	organizations: [MockDefaultOrganization],
	selectedOrganization: MockDefaultOrganization,
	onSelectOrganization: fn(),
	onSaveOverride: fn(),
	isSaving: false,
	...overrides,
});

const organization2OverridesResponse = buildOverridesResponse({
	root: buildOverride("root", {
		mode: "model",
		model_config_id: organization2ModelConfig.id,
		is_set: true,
	}),
});

const MultiOrganizationView = (props: AgentSettingsUserAgentsPageViewProps) => {
	const [selectedOrganization, setSelectedOrganization] = useState(
		MockDefaultOrganization,
	);
	const isOrganization2 = selectedOrganization.id === MockOrganization2.id;
	return (
		<AgentSettingsUserAgentsPageView
			{...props}
			organizations={[MockDefaultOrganization, MockOrganization2]}
			selectedOrganization={selectedOrganization}
			onSelectOrganization={setSelectedOrganization}
			overridesData={
				isOrganization2
					? organization2OverridesResponse
					: buildOverridesResponse()
			}
			models={isOrganization2 ? [organization2ModelConfig] : models}
			modelOptions={isOrganization2 ? [organization2ModelOption] : modelOptions}
		/>
	);
};

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
	excludeStories: [
		"buildArgs",
		"buildOverride",
		"buildOverridesResponse",
		"claudeModelConfig",
		"defaultModelConfig",
		"modelOptions",
		"reasoningModelConfig",
	],
	args: buildArgs(),
} satisfies Meta<typeof AgentSettingsUserAgentsPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsUserAgentsPageView>;

export const EnabledWithNoSavedValues: Story = {
	args: buildArgs(),
};

export const SavingOverride: Story = {
	args: buildArgs({ isSaving: true }),
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
	play: async ({ canvasElement }) => {
		const rootSection = await getSection(canvasElement, "Root agent model");
		await selectOption(
			rootSection,
			canvasElement,
			"Root agent model behavior, GPT 4.1 Mini",
			/GPT-5/i,
		);
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
		await userEvent.click(
			within(rootSection).getByRole("combobox", {
				name: "Root agent model behavior, GPT-5",
			}),
		);
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
};

export const ModelsError: Story = {
	args: buildArgs({
		modelsError: new Error("Failed to load models."),
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
			/Organization default/i,
		);
		await selectOption(
			exploreSection,
			canvasElement,
			"Explore subagent model behavior, Claude Sonnet 4",
			/Chat default/i,
		);
	},
};

export const LoadingState: Story = {
	args: buildArgs({
		overridesData: undefined,
		isLoading: true,
		modelOptions: [],
	}),
};

export const OverridesError: Story = {
	args: buildArgs({
		overridesData: undefined,
		overridesError: new Error("Failed to load overrides"),
	}),
};

export const SwitchOrganizations: Story = {
	args: buildArgs(),
	render: (args) => <MultiOrganizationView {...args} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", {
				name: new RegExp(MockDefaultOrganization.display_name, "i"),
			}),
		);
		await userEvent.click(
			await screen.findByRole("option", {
				name: new RegExp(MockOrganization2.display_name, "i"),
			}),
		);
	},
};

export const NoAvailableOrganizationModels: Story = {
	args: buildArgs({
		modelOptions: [],
		models: [],
		overridesData: buildOverridesResponse({
			root: buildOverride("root", {
				mode: "model",
				model_config_id: "model-stale",
				is_set: true,
			}),
		}),
	}),
};

export const DefaultOrganizationUnresolved: Story = {
	args: buildArgs({
		selectedOrganization: undefined,
		organizations: [],
		modelOptions: [],
		models: [],
	}),
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
};
