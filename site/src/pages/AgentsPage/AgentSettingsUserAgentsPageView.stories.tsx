import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatModel } from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import {
	AgentSettingsUserAgentsPageView,
	type AgentSettingsUserAgentsPageViewProps,
} from "./AgentSettingsUserAgentsPageView";
import type { ModelSelectorOption } from "./components/ChatElements";

const UNAVAILABLE_WARNING =
	"The saved model is unavailable and will be ignored until you choose a valid model override.";

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

const buildOverride = (
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
	models,
	modelsError: undefined,
	isLoadingModels: false,
	organizations: [MockDefaultOrganization],
	selectedOrganization: MockDefaultOrganization,
	onSelectOrganization: fn(),
	isOrganizationUnresolved: false,
	hasNoOrganizationModels: false,
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
			"Organization default: Claude Sonnet 4",
		);
		expect(exploreSection).toHaveTextContent(
			"Organization default: Claude Sonnet 4",
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
			"General subagent model behavior, Organization default: Claude Sonnet 4",
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
	// TODO: This story fails when pixel runs its play function. Fix it and remove the exclude.
	parameters: { pixel: { exclude: true } },
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

		for (const section of [rootSection, generalSection, exploreSection]) {
			expect(
				within(section).getByText("Failed to load models."),
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
			/Organization default/i,
		);
		await selectOption(
			exploreSection,
			canvasElement,
			"Explore subagent model behavior, Claude Sonnet 4",
			/Chat default/i,
		);

		expect(rootSection).toHaveTextContent("Chat default");
		expect(generalSection).toHaveTextContent("Organization default");
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
		const rootSection = await getSection(canvasElement, "Root agent model");
		await expect(
			within(rootSection).getByRole("combobox", {
				name: /Organization Two Model$/,
			}),
		).toBeVisible();
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

export const NoDefaultOrgModels: Story = {
	args: buildArgs({
		hasNoOrganizationModels: true,
		modelOptions: [],
		models: [],
	}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText(/selected organization has no available chat models/i),
		).toBeInTheDocument();
		const rootSection = await getSection(canvasElement, "Root agent model");
		expect(
			within(rootSection).getByRole("button", { name: "Save" }),
		).toBeDisabled();
	},
};

export const DefaultOrganizationUnresolved: Story = {
	args: buildArgs({
		isOrganizationUnresolved: true,
		modelOptions: [],
		models: [],
	}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText(/organization is not available/i),
		).toBeInTheDocument();
		const rootSection = await getSection(canvasElement, "Root agent model");
		expect(
			within(rootSection).getByRole("button", { name: "Save" }),
		).toBeDisabled();
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
		expect(rootSection).toHaveTextContent("Invalid organization default");
		expect(
			within(rootSection).getByText(
				/The saved root override uses the organization default/i,
			),
		).toBeInTheDocument();
		expect(
			within(rootSection).getByRole("button", { name: "Save" }),
		).toBeDisabled();

		await selectOption(
			rootSection,
			canvasElement,
			"Root agent model behavior, Invalid organization default",
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
