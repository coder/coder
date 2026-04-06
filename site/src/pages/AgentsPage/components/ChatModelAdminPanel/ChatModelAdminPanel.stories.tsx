import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ComponentProps, useState } from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import {
	ChatModelAdminPanel,
	type ChatModelAdminSection,
} from "./ChatModelAdminPanel";

// Helpers.

const now = "2026-02-18T12:00:00.000Z";
const nilProviderConfigID = "00000000-0000-0000-0000-000000000000";

const createProviderConfig = (
	overrides: Partial<TypesGen.ChatProviderConfig> &
		Pick<TypesGen.ChatProviderConfig, "id" | "provider">,
): TypesGen.ChatProviderConfig => ({
	id: overrides.id,
	provider: overrides.provider,
	display_name: overrides.display_name ?? "",
	enabled: overrides.enabled ?? true,
	has_api_key: overrides.has_api_key ?? false,
	has_effective_api_key:
		overrides.has_effective_api_key ?? overrides.has_api_key ?? false,
	central_api_key_enabled: overrides.central_api_key_enabled ?? true,
	allow_user_api_key: overrides.allow_user_api_key ?? false,
	allow_central_api_key_fallback:
		overrides.allow_central_api_key_fallback ?? false,
	base_url: overrides.base_url ?? "",
	source: overrides.source ?? "database",
	created_at: overrides.created_at ?? now,
	updated_at: overrides.updated_at ?? now,
});

const createModelConfig = (
	overrides: Partial<TypesGen.ChatModelConfig> &
		Pick<TypesGen.ChatModelConfig, "id" | "provider" | "model">,
): TypesGen.ChatModelConfig => ({
	id: overrides.id,
	provider: overrides.provider,
	model: overrides.model,
	display_name: overrides.display_name ?? overrides.model,
	enabled: overrides.enabled ?? true,
	is_default: overrides.is_default ?? false,
	context_limit: overrides.context_limit ?? 200000,
	compression_threshold: overrides.compression_threshold ?? 70,
	model_config: overrides.model_config,
	created_at: overrides.created_at ?? now,
	updated_at: overrides.updated_at ?? now,
});

const emptyCatalog: TypesGen.ChatModelsResponse = {
	providers: [],
};

const openAIMissingAPIKeyCatalog: TypesGen.ChatModelsResponse = {
	providers: [
		{
			provider: "openai",
			available: false,
			unavailable_reason: "missing_api_key",
			models: [],
		},
	],
};

type ChatModelAdminPanelStoryArgs = ComponentProps<typeof ChatModelAdminPanel>;

const buildProviderStoryArgs = (
	providerConfigsData: TypesGen.ChatProviderConfig[],
	overrides: ChatModelAdminPanelStoryArgs = {},
): ChatModelAdminPanelStoryArgs => ({
	section: "providers" as ChatModelAdminSection,
	providerConfigsData,
	modelConfigsData: [],
	catalogData: emptyCatalog,
	...overrides,
});

const buildModelStoryArgs = (
	providerConfigsData: TypesGen.ChatProviderConfig[],
	modelConfigsData: TypesGen.ChatModelConfig[] = [],
	overrides: ChatModelAdminPanelStoryArgs = {},
): ChatModelAdminPanelStoryArgs => ({
	section: "models" as ChatModelAdminSection,
	providerConfigsData,
	modelConfigsData,
	catalogData: emptyCatalog,
	...overrides,
});

const buildCreatedProviderConfig = (
	req: TypesGen.CreateChatProviderConfigRequest,
	overrides: Partial<TypesGen.ChatProviderConfig> = {},
): TypesGen.ChatProviderConfig => {
	const hasAPIKey = (req.api_key ?? "").trim().length > 0;

	return createProviderConfig({
		display_name: req.display_name ?? overrides.display_name ?? "",
		enabled: overrides.enabled ?? true,
		has_api_key: hasAPIKey,
		has_effective_api_key: hasAPIKey,
		base_url: req.base_url ?? overrides.base_url ?? "",
		source: overrides.source ?? "database",
		...overrides,
		id: overrides.id ?? `provider-${req.provider}`,
		provider: overrides.provider ?? req.provider,
	});
};

const buildUpdatedProviderConfig = (
	current: TypesGen.ChatProviderConfig,
	req: TypesGen.UpdateChatProviderConfigRequest,
): TypesGen.ChatProviderConfig => {
	const hasAPIKey =
		typeof req.api_key === "string"
			? req.api_key.trim().length > 0
			: current.has_api_key;

	return {
		...current,
		display_name:
			typeof req.display_name === "string"
				? req.display_name
				: current.display_name,
		has_api_key: hasAPIKey,
		has_effective_api_key: hasAPIKey,
		base_url:
			typeof req.base_url === "string" ? req.base_url : current.base_url,
		enabled: typeof req.enabled === "boolean" ? req.enabled : current.enabled,
		updated_at: now,
	};
};

const buildCreatedModelConfig = (
	req: TypesGen.CreateChatModelConfigRequest,
	overrides: Partial<TypesGen.ChatModelConfig> = {},
): TypesGen.ChatModelConfig =>
	createModelConfig({
		display_name: req.display_name || overrides.display_name || req.model,
		enabled: req.enabled ?? overrides.enabled ?? true,
		context_limit:
			typeof req.context_limit === "number" &&
			Number.isFinite(req.context_limit)
				? req.context_limit
				: (overrides.context_limit ?? 200000),
		compression_threshold:
			typeof req.compression_threshold === "number" &&
			Number.isFinite(req.compression_threshold)
				? req.compression_threshold
				: (overrides.compression_threshold ?? 70),
		model_config: req.model_config ?? overrides.model_config,
		...overrides,
		id: overrides.id ?? `model-${req.provider}-${req.model}`,
		provider: overrides.provider ?? req.provider,
		model: overrides.model ?? req.model,
	});

const buildUpdatedModelConfig = (
	current: TypesGen.ChatModelConfig,
	req: TypesGen.UpdateChatModelConfigRequest,
): TypesGen.ChatModelConfig =>
	createModelConfig({
		...current,
		...req,
		id: current.id,
		provider: current.provider,
		model: current.model,
		updated_at: now,
	});

const createAndUpdateProviderInitialConfigs = [
	createProviderConfig({
		id: nilProviderConfigID,
		provider: "openai",
		display_name: "OpenAI",
		source: "supported",
		enabled: false,
		has_api_key: false,
	}),
];

const updateModelEnabledToggleProviders = [
	createProviderConfig({
		id: "provider-openai",
		provider: "openai",
		display_name: "OpenAI",
		source: "database",
		has_api_key: true,
	}),
];

const updateModelEnabledToggleInitialConfigs = [
	createModelConfig({
		id: "model-enabled",
		provider: "openai",
		model: "gpt-test-enabled",
		display_name: "GPT Test Enabled",
		enabled: true,
	}),
];

// Meta.

const meta: Meta<typeof ChatModelAdminPanel> = {
	title: "pages/AgentsPage/ChatModelAdminPanel",
	component: ChatModelAdminPanel,
};

export default meta;
type Story = StoryObj<typeof ChatModelAdminPanel>;

// Providers section stories.

export const ProviderAccordionCards: Story = {
	args: buildProviderStoryArgs([
		createProviderConfig({
			id: nilProviderConfigID,
			provider: "openrouter",
			display_name: "OpenRouter",
			source: "supported",
			enabled: false,
		}),
	]),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await expect(await body.findByText("OpenRouter")).toBeInTheDocument();
		expect(body.queryByText("OpenAI")).not.toBeInTheDocument();

		await userEvent.click(body.getByRole("button", { name: /OpenRouter/i }));
		await expect(body.getByLabelText("Base URL")).toBeInTheDocument();
	},
};

export const EnvPresetProviders: Story = {
	args: buildProviderStoryArgs([
		createProviderConfig({
			id: nilProviderConfigID,
			provider: "openai",
			display_name: "OpenAI",
			has_api_key: true,
			source: "env_preset",
			enabled: true,
		}),
		createProviderConfig({
			id: nilProviderConfigID,
			provider: "anthropic",
			display_name: "Anthropic",
			has_api_key: true,
			source: "env_preset",
			enabled: true,
		}),
	]),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await expect(
			await body.findByRole("button", { name: /OpenAI/i }),
		).toBeInTheDocument();
		expect(
			body.getByRole("button", { name: /Anthropic/i }),
		).toBeInTheDocument();

		await userEvent.click(body.getByRole("button", { name: /OpenAI/i }));

		await expect(
			await body.findByText(
				"This provider key is configured from deployment environment settings and cannot be edited in this UI.",
			),
		).toBeVisible();
		expect(body.queryByLabelText(/API key/i)).not.toBeInTheDocument();
		expect(
			body.queryByRole("button", {
				name: "Create provider config",
			}),
		).not.toBeInTheDocument();

		await userEvent.click(body.getByText("Back"));

		await expect(
			await body.findByRole("button", { name: /Anthropic/i }),
		).toBeInTheDocument();

		await userEvent.click(body.getByRole("button", { name: /Anthropic/i }));
		await expect(
			await body.findByText(
				"This provider key is configured from deployment environment settings and cannot be edited in this UI.",
			),
		).toBeVisible();
	},
};

export const CreateAndUpdateProvider: Story = {
	args: buildProviderStoryArgs(createAndUpdateProviderInitialConfigs, {
		catalogData: openAIMissingAPIKeyCatalog,
		onCreateProvider:
			fn<
				(
					req: TypesGen.CreateChatProviderConfigRequest,
				) => Promise<TypesGen.ChatProviderConfig>
			>(),
		onUpdateProvider: fn(),
	}),
	render: function Stateful(args) {
		const [providers, setProviders] = useState(args.providerConfigsData ?? []);

		return (
			<ChatModelAdminPanel
				{...args}
				providerConfigsData={providers}
				onCreateProvider={async (req) => {
					const result = buildCreatedProviderConfig(req, {
						id: "provider-openai-created",
						source: "database",
					});
					setProviders((currentProviders) => [...currentProviders, result]);
					await args.onCreateProvider?.(req);
					return result;
				}}
				onUpdateProvider={async (providerConfigId, req) => {
					const currentProvider = providers.find(
						(provider) => provider.id === providerConfigId,
					);
					if (!currentProvider) {
						throw new Error("Provider config not found.");
					}

					const updatedProvider = buildUpdatedProviderConfig(
						currentProvider,
						req,
					);
					setProviders((currentProviders) =>
						currentProviders.map((provider) =>
							provider.id === providerConfigId ? updatedProvider : provider,
						),
					);
					await args.onUpdateProvider?.(providerConfigId, req);
					return updatedProvider;
				}}
			/>
		);
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByRole("button", { name: /OpenAI/i }));

		await userEvent.type(
			await body.findByLabelText(/API key/i),
			"sk-provider-key",
		);
		await userEvent.type(
			body.getByLabelText("Base URL"),
			"https://proxy.example.com/v1",
		);
		await userEvent.click(
			body.getByRole("button", { name: "Create provider config" }),
		);

		await waitFor(() => {
			expect(args.onCreateProvider).toHaveBeenCalledTimes(1);
		});
		expect(args.onCreateProvider).toHaveBeenCalledWith(
			expect.objectContaining({
				provider: "openai",
				api_key: "sk-provider-key",
				base_url: "https://proxy.example.com/v1",
			}),
		);

		await waitFor(() => {
			expect(
				body.getByRole("button", { name: "Save changes" }),
			).toBeInTheDocument();
		});

		const apiKeyInput = body.getByLabelText(/API key/i);
		await userEvent.clear(apiKeyInput);
		await userEvent.type(apiKeyInput, "sk-updated-provider-key");
		const baseURLInput = body.getByLabelText("Base URL");
		await userEvent.clear(baseURLInput);
		await userEvent.type(baseURLInput, "https://internal-proxy.example.com/v2");
		await userEvent.click(body.getByRole("button", { name: "Save changes" }));

		await waitFor(() => {
			expect(args.onUpdateProvider).toHaveBeenCalledTimes(1);
		});
		expect(args.onUpdateProvider).toHaveBeenCalledWith(
			expect.any(String),
			expect.objectContaining({
				api_key: "sk-updated-provider-key",
				base_url: "https://internal-proxy.example.com/v2",
			}),
		);
	},
};

export const MultipleProviderConfigsSameFamily: Story = {
	args: buildProviderStoryArgs([
		createProviderConfig({
			id: "provider-openai-1",
			provider: "openai",
			display_name: "OpenAI Production",
			source: "database",
			has_api_key: true,
			enabled: true,
		}),
		createProviderConfig({
			id: "provider-openai-2",
			provider: "openai",
			display_name: "OpenAI Staging",
			source: "database",
			has_api_key: true,
			enabled: false,
		}),
	]),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await expect(
			await body.findByText("OpenAI Production"),
		).toBeInTheDocument();
		expect(body.getByText("OpenAI Staging")).toBeInTheDocument();
	},
};

export const AddProviderConfigFlow: Story = {
	args: buildProviderStoryArgs(
		[
			createProviderConfig({
				id: "provider-openai",
				provider: "openai",
				display_name: "OpenAI",
				source: "database",
				has_api_key: true,
			}),
		],
		{ sectionLabel: "Providers" },
	),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await expect(await body.findByText("OpenAI")).toBeInTheDocument();

		const addButton = body.getByRole("button", { name: "Add provider" });
		await userEvent.click(addButton);

		const item = await body.findByRole("menuitem", { name: /OpenAI/i });
		await userEvent.click(item);

		await expect(await body.findByLabelText(/API key/i)).toBeInTheDocument();
		expect(
			body.getByRole("button", { name: "Create provider config" }),
		).toBeInTheDocument();
	},
};

const providerEnabledToggleConfig = createProviderConfig({
	id: "provider-openai",
	provider: "openai",
	display_name: "OpenAI",
	source: "database",
	has_api_key: true,
	enabled: true,
});

export const ProviderEnabledToggle: Story = {
	args: buildProviderStoryArgs([providerEnabledToggleConfig], {
		onUpdateProvider: fn(async (_providerConfigId, req) =>
			buildUpdatedProviderConfig(providerEnabledToggleConfig, req),
		),
	}),
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByRole("button", { name: /OpenAI/i }));

		const toggle = await body.findByRole("switch");
		await expect(toggle).toBeChecked();

		await userEvent.click(toggle);

		await waitFor(() => {
			expect(args.onUpdateProvider).toHaveBeenCalledWith(
				"provider-openai",
				expect.objectContaining({ enabled: false }),
			);
		});
	},
};

// Models section stories.

const openAddModelForm = async (
	body: ReturnType<typeof within>,
	providerLabel: string,
) => {
	const triggers = await body.findAllByRole("button", { name: "Add model" });
	await userEvent.click(triggers[0]);

	const item = await body.findByText(new RegExp(`^${providerLabel}$`, "i"));
	await userEvent.click(item);

	await body.findByLabelText(/Model Identifier/i);
};

const openAIModelProviderConfig = createProviderConfig({
	id: "provider-openai",
	provider: "openai",
	display_name: "OpenAI",
	source: "database",
	has_api_key: true,
});

export const NoModelConfigByDefault: Story = {
	args: buildModelStoryArgs([openAIModelProviderConfig], [], {
		onCreateModel: fn(async (req) => buildCreatedModelConfig(req)),
	}),
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await openAddModelForm(body, "OpenAI");

		await userEvent.type(body.getByLabelText(/Model Identifier/i), "gpt-5-pro");
		await userEvent.type(body.getByLabelText(/Context limit/i), "200000");

		await userEvent.click(body.getByText("Advanced"));
		await expect(await body.findByLabelText(/Max output tokens/i)).toHaveValue(
			"",
		);

		await userEvent.click(body.getByRole("button", { name: "Add model" }));
		await waitFor(() => {
			expect(args.onCreateModel).toHaveBeenCalledTimes(1);
		});
		expect(args.onCreateModel).toHaveBeenCalledWith(
			expect.objectContaining({
				provider: "openai",
				model: "gpt-5-pro",
			}),
		);
		expect(args.onCreateModel).toHaveBeenCalledWith(
			expect.not.objectContaining({
				model_config: expect.anything(),
			}),
		);
	},
};

export const SubmitModelConfigExplicitly: Story = {
	args: buildModelStoryArgs([openAIModelProviderConfig], [], {
		onCreateModel: fn(async (req) => buildCreatedModelConfig(req)),
	}),
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await openAddModelForm(body, "OpenAI");

		await userEvent.type(
			body.getByLabelText(/Model Identifier/i),
			"gpt-5-pro-custom",
		);
		await userEvent.type(body.getByLabelText(/Context limit/i), "200000");
		await userEvent.click(body.getByText("Advanced"));
		await userEvent.type(
			await body.findByLabelText(/Max output tokens/i),
			"32000",
		);
		await userEvent.click(
			body.getByRole("combobox", {
				name: "Reasoning Effort",
			}),
		);
		await userEvent.click(await body.findByRole("option", { name: "high" }));

		await userEvent.click(body.getByRole("button", { name: "Add model" }));
		await waitFor(() => {
			expect(args.onCreateModel).toHaveBeenCalledTimes(1);
		});
		expect(args.onCreateModel).toHaveBeenCalledWith(
			expect.objectContaining({
				provider: "openai",
				model: "gpt-5-pro-custom",
				model_config: expect.objectContaining({
					max_output_tokens: 32000,
					provider_options: {
						openai: {
							reasoning_effort: "high",
						},
					},
				}),
			}),
		);
	},
};

export const UpdateModelEnabledToggle: Story = {
	args: buildModelStoryArgs(
		updateModelEnabledToggleProviders,
		updateModelEnabledToggleInitialConfigs,
		{
			onUpdateModel: fn(),
		},
	),
	render: function Stateful(args) {
		const [modelConfigs, setModelConfigs] = useState(
			args.modelConfigsData ?? [],
		);

		return (
			<ChatModelAdminPanel
				{...args}
				modelConfigsData={modelConfigs}
				onUpdateModel={async (modelConfigId, req) => {
					const currentModelConfig = modelConfigs.find(
						(modelConfig) => modelConfig.id === modelConfigId,
					);
					if (!currentModelConfig) {
						throw new Error("Model config not found.");
					}

					const updatedModelConfig = buildUpdatedModelConfig(
						currentModelConfig,
						req,
					);
					setModelConfigs((currentModelConfigs) =>
						currentModelConfigs.map((modelConfig) =>
							modelConfig.id === modelConfigId
								? updatedModelConfig
								: modelConfig,
						),
					);
					await args.onUpdateModel?.(modelConfigId, req);
					return updatedModelConfig;
				}}
			/>
		);
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByText("GPT Test Enabled"));

		const enabledSwitch = await body.findByRole("switch", { name: "Enabled" });
		await expect(enabledSwitch).toBeChecked();
		await userEvent.click(enabledSwitch);
		await expect(enabledSwitch).not.toBeChecked();

		await userEvent.click(body.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(args.onUpdateModel).toHaveBeenCalledTimes(1);
		});
		expect(args.onUpdateModel).toHaveBeenCalledWith(
			"model-enabled",
			expect.objectContaining({ enabled: false }),
		);

		const modelRow = await body.findByRole("button", {
			name: /gpt test enabled/i,
		});
		await expect(within(modelRow).getByText("disabled")).toBeVisible();
	},
};

const multiConfigProviders = [
	createProviderConfig({
		id: "provider-openai-prod",
		provider: "openai",
		display_name: "OpenAI Production",
		source: "database",
		has_api_key: true,
	}),
	createProviderConfig({
		id: "provider-openai-dev",
		provider: "openai",
		display_name: "",
		source: "database",
		has_api_key: true,
	}),
];

export const CreateModelMultiConfig: Story = {
	args: buildModelStoryArgs(multiConfigProviders, [], {
		onCreateModel: fn(async (req) => buildCreatedModelConfig(req)),
	}),
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		const triggers = await body.findAllByRole("button", { name: "Add model" });
		await userEvent.click(triggers[0]);

		await expect(
			await body.findByText(/^OpenAI Production$/i),
		).toBeInTheDocument();
		const secondaryOption = await body.findByText(/^OpenAI 2$/i);
		await expect(secondaryOption).toBeInTheDocument();

		await userEvent.click(secondaryOption);
		await body.findByLabelText(/Model Identifier/i);
		await expect(body.getByText(/^OpenAI 2$/i)).toBeVisible();

		await userEvent.type(body.getByLabelText(/Model Identifier/i), "gpt-5-pro");
		await userEvent.type(body.getByLabelText(/Context limit/i), "200000");

		const submitButtons = await body.findAllByRole("button", {
			name: "Add model",
		});
		await userEvent.click(submitButtons[submitButtons.length - 1]);

		await waitFor(() => {
			expect(args.onCreateModel).toHaveBeenCalledWith(
				expect.objectContaining({
					provider: "openai",
					provider_config_id: "provider-openai-dev",
				}),
			);
		});
	},
};

// Per-provider model form stories.
// Each story opens the Add model form for a specific provider.

const providerFormSetup = (provider: string, displayName: string) => ({
	args: buildModelStoryArgs([
		createProviderConfig({
			id: `provider-${provider}`,
			provider,
			display_name: displayName,
			source: "database",
			has_api_key: true,
		}),
	]),
});

export const ModelFormOpenAI: Story = {
	...providerFormSetup("openai", "OpenAI"),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");
		await expect(
			await body.findByLabelText(/Reasoning Effort/i),
		).toBeInTheDocument();
		await expect(
			await body.findByLabelText(/Parallel Tool Calls/i),
		).toBeInTheDocument();
	},
};

export const ModelFormAnthropic: Story = {
	...providerFormSetup("anthropic", "Anthropic"),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "Anthropic");
		await expect(
			await body.findByLabelText(/Send Reasoning/i),
		).toBeInTheDocument();
		await expect(
			await body.findByLabelText(/Thinking Budget Tokens/i),
		).toBeInTheDocument();
	},
};

export const ModelFormGoogle: Story = {
	...providerFormSetup("google", "Google"),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "Google");
		await expect(
			await body.findByLabelText(/Thinking Config Thinking Budget/i),
		).toBeInTheDocument();
		await expect(
			await body.findByLabelText(/Thinking Config Include Thoughts/i),
		).toBeInTheDocument();
	},
};

export const ModelFormOpenAICompat: Story = {
	...providerFormSetup("openaicompat", "OpenAI-compatible"),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI-compatible");
		await expect(
			await body.findByLabelText(/Reasoning Effort/i),
		).toBeInTheDocument();
	},
};

export const ModelFormOpenRouter: Story = {
	...providerFormSetup("openrouter", "OpenRouter"),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenRouter");
		await expect(
			await body.findByLabelText(/Reasoning Enabled/i),
		).toBeInTheDocument();
		await expect(
			await body.findByLabelText(/Reasoning Max Tokens/i),
		).toBeInTheDocument();
	},
};

export const ModelFormVercel: Story = {
	...providerFormSetup("vercel", "Vercel AI Gateway"),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "Vercel AI Gateway");
		await expect(
			await body.findByLabelText(/Reasoning Enabled/i),
		).toBeInTheDocument();
		await expect(
			await body.findByLabelText(/Parallel Tool Calls/i),
		).toBeInTheDocument();
	},
};

export const ModelFormAzure: Story = {
	...providerFormSetup("azure", "Azure OpenAI"),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "Azure OpenAI");
		await expect(
			await body.findByLabelText(/Reasoning Effort/i),
		).toBeInTheDocument();
		await expect(
			await body.findByLabelText(/Service Tier/i),
		).toBeInTheDocument();
	},
};

export const ModelFormBedrock: Story = {
	...providerFormSetup("bedrock", "AWS Bedrock"),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "AWS Bedrock");
		await expect(
			await body.findByLabelText(/Send Reasoning/i),
		).toBeInTheDocument();
		await expect(await body.findByLabelText(/Effort/i)).toBeInTheDocument();
	},
};

export const ModelPricingWarningInList: Story = {
	args: buildModelStoryArgs(
		[openAIModelProviderConfig],
		[
			createModelConfig({
				id: "model-warning",
				provider: "openai",
				model: "gpt-4.1",
				display_name: "GPT-4.1",
			}),
		],
	),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await expect(await body.findByText("GPT-4.1")).toBeInTheDocument();
		await expect(
			body.getByText("Model pricing is not defined"),
		).toBeInTheDocument();
	},
};

const deletableOpenAIProviderConfig = createProviderConfig({
	id: "provider-openai",
	provider: "openai",
	display_name: "OpenAI",
	source: "database",
	has_api_key: true,
});

const deletableGPT4oModelConfig = createModelConfig({
	id: "model-1",
	provider: "openai",
	model: "gpt-4o",
	display_name: "GPT-4o",
});

export const ModelDeleteConfirmation: Story = {
	args: buildModelStoryArgs(
		[deletableOpenAIProviderConfig],
		[deletableGPT4oModelConfig],
	),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByText("GPT-4o"));
		await expect(
			await body.findByLabelText(/Model Identifier/i),
		).toBeInTheDocument();

		const deleteButton = await body.findByRole("button", { name: "Delete" });
		await expect(deleteButton).toBeInTheDocument();
		await userEvent.click(deleteButton);

		await expect(
			await body.findByText(/Are you sure you want to delete this model/i),
		).toBeInTheDocument();
		await expect(body.getByRole("dialog")).toBeInTheDocument();
		await expect(
			body.getByRole("button", { name: "Delete model" }),
		).toBeInTheDocument();
		await expect(
			body.getByRole("button", { name: "Cancel" }),
		).toBeInTheDocument();
	},
};

export const ModelDeleteCancelled: Story = {
	args: buildModelStoryArgs(
		[deletableOpenAIProviderConfig],
		[deletableGPT4oModelConfig],
	),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByText("GPT-4o"));
		await userEvent.click(await body.findByRole("button", { name: "Delete" }));
		await body.findByText(/Are you sure/i);
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		await waitFor(() => {
			expect(body.queryByRole("dialog")).not.toBeInTheDocument();
		});
	},
};

export const ModelDeleteConfirmed: Story = {
	args: buildModelStoryArgs(
		[deletableOpenAIProviderConfig],
		[deletableGPT4oModelConfig],
		{
			onDeleteModel: fn(async () => undefined),
		},
	),
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByText("GPT-4o"));
		await userEvent.click(await body.findByRole("button", { name: "Delete" }));
		await userEvent.click(
			await body.findByRole("button", { name: "Delete model" }),
		);

		await waitFor(() => {
			expect(args.onDeleteModel).toHaveBeenCalledTimes(1);
		});
		expect(args.onDeleteModel).toHaveBeenCalledWith("model-1");
	},
};

export const ProviderDeleteConfirmation: Story = {
	args: buildProviderStoryArgs([deletableOpenAIProviderConfig]),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByRole("button", { name: /OpenAI/i }));

		const deleteButton = await body.findByRole("button", { name: "Delete" });
		await userEvent.click(deleteButton);

		await expect(
			await body.findByText(/Are you sure you want to delete this provider/i),
		).toBeInTheDocument();
		await expect(body.getByRole("dialog")).toBeInTheDocument();
		await expect(
			body.getByRole("button", { name: "Delete provider" }),
		).toBeInTheDocument();
		await expect(
			body.getByRole("button", { name: "Cancel" }),
		).toBeInTheDocument();
	},
};

export const ProviderDeleteCancelled: Story = {
	args: buildProviderStoryArgs([deletableOpenAIProviderConfig]),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByRole("button", { name: /OpenAI/i }));
		await userEvent.click(await body.findByRole("button", { name: "Delete" }));
		await body.findByText(/Are you sure/i);
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		await waitFor(() => {
			expect(body.queryByRole("dialog")).not.toBeInTheDocument();
		});
	},
};

export const ProviderDeleteConfirmed: Story = {
	args: buildProviderStoryArgs([deletableOpenAIProviderConfig], {
		onDeleteProvider: fn(async () => undefined),
	}),
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByRole("button", { name: /OpenAI/i }));
		await userEvent.click(await body.findByRole("button", { name: "Delete" }));
		await userEvent.click(
			await body.findByRole("button", { name: "Delete provider" }),
		);

		await waitFor(() => {
			expect(args.onDeleteProvider).toHaveBeenCalledTimes(1);
		});
		expect(args.onDeleteProvider).toHaveBeenCalledWith("provider-openai");
	},
};

export const ValidatesModelConfigFields: Story = {
	args: buildModelStoryArgs([openAIModelProviderConfig], [], {
		onCreateModel: fn(async (req) => buildCreatedModelConfig(req)),
	}),
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await openAddModelForm(body, "OpenAI");

		await userEvent.type(body.getByLabelText(/Model Identifier/i), "gpt-5-pro");
		await userEvent.type(body.getByLabelText(/Context limit/i), "200000");
		await userEvent.click(body.getByText("Advanced"));
		const maxOutputTokensInput =
			await body.findByLabelText(/Max output tokens/i);
		await userEvent.type(maxOutputTokensInput, "not-a-number");
		await waitFor(() => {
			expect(body.getByRole("button", { name: "Add model" })).toBeDisabled();
		});
		expect(args.onCreateModel).not.toHaveBeenCalled();
	},
};
