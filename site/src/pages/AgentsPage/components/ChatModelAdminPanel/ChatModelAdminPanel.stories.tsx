import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ComponentProps, useState } from "react";
import {
	expect,
	fireEvent,
	fn,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	ChatModelAdminPanel,
	type ChatModelAdminSection,
} from "./ChatModelAdminPanel";
import { formatContextBadge, getKnownModelsForProvider } from "./knownModels";

// ── Helpers ────────────────────────────────────────────────────

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

type ChatModelAdminPanelStoryProps = ComponentProps<typeof ChatModelAdminPanel>;

/**
 * Set up spies for all chat admin API methods. The mutable `state`
 * object lets mutation spies update what queries return on refetch,
 * mimicking the real server round-trip.
 */
const setupChatSpies = (state: {
	providerConfigs: TypesGen.ChatProviderConfig[];
	modelConfigs: TypesGen.ChatModelConfig[];
	modelCatalog: TypesGen.ChatModelsResponse;
}) => {
	spyOn(API.experimental, "getChatProviderConfigs").mockImplementation(
		async () => {
			return state.providerConfigs;
		},
	);
	spyOn(API.experimental, "getChatModelConfigs").mockImplementation(
		async () => {
			return state.modelConfigs;
		},
	);
	spyOn(API.experimental, "getChatModels").mockImplementation(async () => {
		return state.modelCatalog;
	});

	spyOn(API.experimental, "createChatProviderConfig").mockImplementation(
		async (req) => {
			const created = createProviderConfig({
				id: `provider-${Date.now()}`,
				provider: req.provider,
				display_name: req.display_name ?? "",
				has_api_key: (req.api_key ?? "").trim().length > 0,
				central_api_key_enabled: req.central_api_key_enabled ?? true,
				allow_user_api_key: req.allow_user_api_key ?? false,
				allow_central_api_key_fallback:
					req.allow_central_api_key_fallback ?? false,
				base_url: req.base_url ?? "",
				source: "database",
			});
			state.providerConfigs = [
				...state.providerConfigs.filter((p) => p.provider !== req.provider),
				created,
			];
			return created;
		},
	);

	spyOn(API.experimental, "updateChatProviderConfig").mockImplementation(
		async (providerConfigId, req) => {
			const idx = state.providerConfigs.findIndex(
				(p) => p.id === providerConfigId,
			);
			if (idx < 0) {
				throw new Error("Provider config not found.");
			}
			const current = state.providerConfigs[idx];
			const updated: TypesGen.ChatProviderConfig = {
				...current,
				display_name:
					typeof req.display_name === "string"
						? req.display_name
						: current.display_name,
				has_api_key:
					typeof req.api_key === "string"
						? req.api_key.trim().length > 0
						: current.has_api_key,
				central_api_key_enabled:
					typeof req.central_api_key_enabled === "boolean"
						? req.central_api_key_enabled
						: current.central_api_key_enabled,
				allow_user_api_key:
					typeof req.allow_user_api_key === "boolean"
						? req.allow_user_api_key
						: current.allow_user_api_key,
				allow_central_api_key_fallback:
					typeof req.allow_central_api_key_fallback === "boolean"
						? req.allow_central_api_key_fallback
						: current.allow_central_api_key_fallback,
				base_url:
					typeof req.base_url === "string" ? req.base_url : current.base_url,
				updated_at: now,
			};
			state.providerConfigs = state.providerConfigs.map((p, i) =>
				i === idx ? updated : p,
			);
			return updated;
		},
	);

	spyOn(API.experimental, "createChatModelConfig").mockImplementation(
		async (req) => {
			const created = createModelConfig({
				id: `model-${state.modelConfigs.length + 1}`,
				provider: req.provider,
				model: req.model,
				display_name: req.display_name || req.model,
				enabled: req.enabled ?? true,
				context_limit:
					typeof req.context_limit === "number" &&
					Number.isFinite(req.context_limit)
						? req.context_limit
						: 200000,
				compression_threshold:
					typeof req.compression_threshold === "number" &&
					Number.isFinite(req.compression_threshold)
						? req.compression_threshold
						: 70,
				model_config: req.model_config,
			});
			state.modelConfigs = [...state.modelConfigs, created];
			return created;
		},
	);

	spyOn(API.experimental, "deleteChatModelConfig").mockImplementation(
		async (modelConfigId) => {
			state.modelConfigs = state.modelConfigs.filter(
				(m) => m.id !== modelConfigId,
			);
		},
	);

	// Unused but mock to avoid errors.
	spyOn(API.experimental, "deleteChatProviderConfig").mockResolvedValue(
		undefined,
	);
	spyOn(API.experimental, "updateChatModelConfig").mockImplementation(
		async (modelConfigId, req) => {
			const idx = state.modelConfigs.findIndex((m) => m.id === modelConfigId);
			if (idx < 0) {
				throw new Error("Model config not found.");
			}

			const current = state.modelConfigs[idx];
			const updated = createModelConfig({
				...current,
				...req,
				id: current.id,
				provider: current.provider,
				model: current.model,
				updated_at: now,
			});

			state.modelConfigs = state.modelConfigs.map((modelConfig, i) =>
				i === idx ? updated : modelConfig,
			);

			return updated;
		},
	);
};

// ── Meta ───────────────────────────────────────────────────────

const meta: Meta<typeof ChatModelAdminPanel> = {
	title: "pages/AgentsPage/ChatModelAdminPanel",
	component: ChatModelAdminPanel,
	args: {
		providerConfigsData: [],
		modelConfigsData: [],
		modelCatalogData: { providers: [] },
		isLoading: false,
		providerConfigsError: null,
		modelConfigsError: null,
		modelCatalogError: null,
		onCreateProvider: fn(async () => ({})),
		onUpdateProvider: fn(async () => ({})),
		onDeleteProvider: fn(async () => undefined),
		isProviderMutationPending: false,
		providerMutationError: null,
		onCreateModel: fn(async () => ({})),
		onUpdateModel: fn(async () => ({})),
		onDeleteModel: fn(async () => undefined),
		isCreatingModel: false,
		isUpdatingModel: false,
		isDeletingModel: false,
		modelMutationError: null,
	},
};

export default meta;
type Story = StoryObj<typeof ChatModelAdminPanel>;

// ── Providers section stories ──────────────────────────────────

export const ProviderAccordionCards: Story = {
	args: {
		section: "providers" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: nilProviderConfigID,
				provider: "openrouter",
				display_name: "OpenRouter",
				source: "supported",
				enabled: false,
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await expect(await body.findByText("OpenRouter")).toBeInTheDocument();
		// OpenAI should not be rendered.
		expect(body.queryByText("OpenAI")).not.toBeInTheDocument();

		await userEvent.click(body.getByRole("button", { name: /OpenRouter/i }));
		await expect(await body.findByLabelText("Base URL")).toBeInTheDocument();
	},
};

export const EnvPresetProviders: Story = {
	args: {
		section: "providers" as ChatModelAdminSection,
		providerConfigsData: [
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
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Both providers should be visible in the list.
		await expect(
			await body.findByRole("button", { name: /OpenAI/i }),
		).toBeInTheDocument();
		expect(
			body.getByRole("button", { name: /Anthropic/i }),
		).toBeInTheDocument();

		// Navigate to OpenAI detail view.
		await userEvent.click(body.getByRole("button", { name: /OpenAI/i }));

		// In the detail view we should see the env-managed alert.
		await expect(
			await body.findByText(
				"This provider key is configured from deployment environment settings and cannot be edited in this UI.",
			),
		).toBeVisible();
		// No API key input or create button should be present.
		expect(body.queryByLabelText(/^API Key$/i)).not.toBeInTheDocument();
		expect(
			body.queryByRole("button", {
				name: "Create provider config",
			}),
		).not.toBeInTheDocument();

		// Navigate back to the list.
		await userEvent.click(body.getByText("Back"));

		// Verify Anthropic is visible in the list again.
		await expect(
			await body.findByRole("button", { name: /Anthropic/i }),
		).toBeInTheDocument();

		// Navigate to Anthropic detail view and verify it's also env-managed.
		await userEvent.click(
			await body.findByRole("button", { name: /Anthropic/i }),
		);
		await expect(
			await body.findByText(
				"This provider key is configured from deployment environment settings and cannot be edited in this UI.",
			),
		).toBeVisible();
	},
};

export const CreateAndUpdateProvider: Story = {
	render: function CreateAndUpdateProvider(args) {
		const [providerConfigsData, setProviderConfigsData] = useState(
			args.providerConfigsData,
		);

		const handleCreateProvider: ChatModelAdminPanelStoryProps["onCreateProvider"] =
			async (req) => {
				const result = await args.onCreateProvider(req);
				const created = createProviderConfig({
					id: `provider-${Date.now()}`,
					provider: req.provider,
					display_name: req.display_name ?? "",
					has_api_key: (req.api_key ?? "").trim().length > 0,
					central_api_key_enabled: req.central_api_key_enabled ?? true,
					allow_user_api_key: req.allow_user_api_key ?? false,
					allow_central_api_key_fallback:
						req.allow_central_api_key_fallback ?? false,
					base_url: req.base_url ?? "",
					source: "database",
				});
				setProviderConfigsData((current) => [
					...(current ?? []).filter((p) => p.provider !== req.provider),
					created,
				]);
				return result;
			};

		const handleUpdateProvider: ChatModelAdminPanelStoryProps["onUpdateProvider"] =
			async (providerConfigId, req) => {
				const result = await args.onUpdateProvider(providerConfigId, req);
				setProviderConfigsData((current) => {
					if (!current) {
						return current;
					}
					return current.map((providerConfig) =>
						providerConfig.id === providerConfigId
							? {
									...providerConfig,
									display_name:
										typeof req.display_name === "string"
											? req.display_name
											: providerConfig.display_name,
									has_api_key:
										typeof req.api_key === "string"
											? req.api_key.trim().length > 0
											: providerConfig.has_api_key,
									central_api_key_enabled:
										typeof req.central_api_key_enabled === "boolean"
											? req.central_api_key_enabled
											: providerConfig.central_api_key_enabled,
									allow_user_api_key:
										typeof req.allow_user_api_key === "boolean"
											? req.allow_user_api_key
											: providerConfig.allow_user_api_key,
									allow_central_api_key_fallback:
										typeof req.allow_central_api_key_fallback === "boolean"
											? req.allow_central_api_key_fallback
											: providerConfig.allow_central_api_key_fallback,
									base_url:
										typeof req.base_url === "string"
											? req.base_url
											: providerConfig.base_url,
									updated_at: "2026-02-18T12:00:00.000Z",
								}
							: providerConfig,
					);
				});
				return result;
			};

		return (
			<ChatModelAdminPanel
				{...args}
				providerConfigsData={providerConfigsData}
				onCreateProvider={handleCreateProvider}
				onUpdateProvider={handleUpdateProvider}
			/>
		);
	},
	args: {
		section: "providers" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: nilProviderConfigID,
				provider: "openai",
				display_name: "OpenAI",
				source: "supported",
				enabled: false,
				has_api_key: false,
			}),
		],
		modelCatalogData: {
			providers: [
				{
					provider: "openai",
					available: false,
					unavailable_reason: "missing_api_key",
					models: [],
				},
			],
		},
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(await body.findByRole("button", { name: /OpenAI/i }));

		await expect(
			await body.findByRole("switch", { name: "Central API key" }),
		).toBeChecked();
		expect(
			await body.findByRole("switch", { name: "Allow user API keys" }),
		).not.toBeChecked();
		expect(
			body.queryByRole("switch", { name: "Use central key as fallback" }),
		).not.toBeInTheDocument();

		await userEvent.type(
			await body.findByLabelText(/^API Key$/i),
			"sk-provider-key",
		);
		await userEvent.type(
			await body.findByLabelText("Base URL"),
			"https://proxy.example.com/v1",
		);
		await userEvent.click(
			await body.findByRole("button", { name: "Create provider config" }),
		);

		await waitFor(() => {
			expect(args.onCreateProvider).toHaveBeenCalledTimes(1);
		});
		await waitFor(() => {
			expect(body.getByRole("button", { name: "Save changes" })).toBeDisabled();
		});
		expect(args.onCreateProvider).toHaveBeenCalledWith(
			expect.objectContaining({
				provider: "openai",
				api_key: "sk-provider-key",
				base_url: "https://proxy.example.com/v1",
				central_api_key_enabled: true,
				allow_user_api_key: false,
				allow_central_api_key_fallback: false,
			}),
		);

		await waitFor(() => {
			expect(
				body.getByRole("button", { name: "Save changes" }),
			).toBeInTheDocument();
		});

		await userEvent.click(
			await body.findByRole("switch", { name: "Allow user API keys" }),
		);
		await userEvent.click(
			await body.findByRole("switch", { name: "Use central key as fallback" }),
		);

		const apiKeyInput = body.getByLabelText(/^API Key$/i);
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
				allow_user_api_key: true,
				allow_central_api_key_fallback: true,
			}),
		);
	},
};

export const ProviderWithUserKeysEnabled: Story = {
	args: {
		section: "providers" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-openai-user-keys",
				provider: "openai",
				display_name: "OpenAI",
				has_api_key: true,
				central_api_key_enabled: true,
				allow_user_api_key: true,
				allow_central_api_key_fallback: false,
			}),
		],
		modelCatalogData: { providers: [] },
	},
	beforeEach: () => {
		setupChatSpies({
			providerConfigs: [
				createProviderConfig({
					id: "provider-openai-user-keys",
					provider: "openai",
					display_name: "OpenAI",
					has_api_key: true,
					central_api_key_enabled: true,
					allow_user_api_key: true,
					allow_central_api_key_fallback: false,
				}),
			],
			modelConfigs: [],
			modelCatalog: { providers: [] },
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await expect(
			await body.findByText("User keys enabled"),
		).toBeInTheDocument();
		await userEvent.click(body.getByRole("button", { name: /OpenAI/i }));
		await expect(
			await body.findByRole("switch", { name: "Allow user API keys" }),
		).toBeChecked();
		await expect(
			await body.findByRole("switch", {
				name: "Use central key as fallback",
			}),
		).not.toBeChecked();
	},
};

export const ProviderWithCentralFallback: Story = {
	args: {
		section: "providers" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-openrouter-fallback",
				provider: "openrouter",
				display_name: "OpenRouter",
				has_api_key: true,
				central_api_key_enabled: true,
				allow_user_api_key: true,
				allow_central_api_key_fallback: true,
			}),
		],
		modelCatalogData: { providers: [] },
	},
	beforeEach: () => {
		setupChatSpies({
			providerConfigs: [
				createProviderConfig({
					id: "provider-openrouter-fallback",
					provider: "openrouter",
					display_name: "OpenRouter",
					has_api_key: true,
					central_api_key_enabled: true,
					allow_user_api_key: true,
					allow_central_api_key_fallback: true,
				}),
			],
			modelConfigs: [],
			modelCatalog: { providers: [] },
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("button", { name: /OpenRouter/i }),
		);
		await expect(
			await body.findByRole("switch", {
				name: "Use central key as fallback",
			}),
		).toBeChecked();
	},
};

export const ProviderWithUserKeysOnly: Story = {
	args: {
		section: "providers" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-google-user-only",
				provider: "google",
				display_name: "Google",
				has_api_key: false,
				central_api_key_enabled: false,
				allow_user_api_key: true,
				allow_central_api_key_fallback: false,
			}),
		],
		modelCatalogData: { providers: [] },
	},
	beforeEach: () => {
		setupChatSpies({
			providerConfigs: [
				createProviderConfig({
					id: "provider-google-user-only",
					provider: "google",
					display_name: "Google",
					has_api_key: false,
					central_api_key_enabled: false,
					allow_user_api_key: true,
					allow_central_api_key_fallback: false,
				}),
			],
			modelConfigs: [],
			modelCatalog: { providers: [] },
		});
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(await body.findByRole("button", { name: /Google/i }));
		await expect(
			await body.findByRole("switch", { name: "Central API key" }),
		).not.toBeChecked();
		await expect(
			await body.findByRole("switch", { name: "Allow user API keys" }),
		).toBeChecked();
		expect(body.queryByLabelText(/^API Key$/i)).not.toBeInTheDocument();
		expect(
			body.queryByRole("switch", { name: "Use central key as fallback" }),
		).not.toBeInTheDocument();

		const saveButton = body.getByRole("button", { name: "Save changes" });
		await userEvent.click(
			body.getByRole("switch", { name: "Central API key" }),
		);
		await expect(await body.findByLabelText(/^API Key$/i)).toBeRequired();
		expect(saveButton).toBeDisabled();

		await userEvent.type(
			body.getByLabelText(/^API Key$/i),
			"sk-google-central-key",
		);
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onUpdateProvider).toHaveBeenCalledTimes(1);
		});
		expect(args.onUpdateProvider).toHaveBeenCalledWith(
			"provider-google-user-only",
			expect.objectContaining({
				api_key: "sk-google-central-key",
				central_api_key_enabled: true,
			}),
		);
	},
};

export const ProviderApiKeyInputMasked: Story = {
	args: {
		section: "providers" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-openai-mask",
				provider: "openai",
				display_name: "OpenAI",
				has_api_key: true,
			}),
		],
		modelCatalogData: { providers: [] },
	},
	beforeEach: () => {
		setupChatSpies({
			providerConfigs: [
				createProviderConfig({
					id: "provider-openai-mask",
					provider: "openai",
					display_name: "OpenAI",
					has_api_key: true,
				}),
			],
			modelConfigs: [],
			modelCatalog: { providers: [] },
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(await body.findByRole("button", { name: /OpenAI/i }));
		await expect(await body.findByLabelText(/^API Key$/i)).toHaveAttribute(
			"type",
			"password",
		);
	},
};

export const ModelFormUserKeyOnlyProvider: Story = {
	args: {
		section: "models" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-google-user-only-models",
				provider: "google",
				display_name: "Google",
				has_api_key: false,
				central_api_key_enabled: false,
				allow_user_api_key: true,
				allow_central_api_key_fallback: false,
			}),
		],
		modelCatalogData: { providers: [] },
	},
	beforeEach: () => {
		setupChatSpies({
			providerConfigs: [
				createProviderConfig({
					id: "provider-google-user-only-models",
					provider: "google",
					display_name: "Google",
					has_api_key: false,
					central_api_key_enabled: false,
					allow_user_api_key: true,
					allow_central_api_key_fallback: false,
				}),
			],
			modelConfigs: [],
			modelCatalog: { providers: [] },
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "Google");
		await expect(
			await body.findByLabelText(/Model Identifier/i),
		).toBeInTheDocument();
		expect(
			body.queryByText(
				"Set an API key for this provider on the Providers tab before adding models.",
			),
		).not.toBeInTheDocument();
	},
};

export const ProviderInvalidCredentialState: Story = {
	args: {
		section: "providers" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-bedrock-invalid",
				provider: "bedrock",
				display_name: "Bedrock",
				has_api_key: false,
				central_api_key_enabled: false,
				allow_user_api_key: false,
				allow_central_api_key_fallback: false,
			}),
		],
		modelCatalogData: { providers: [] },
	},
	beforeEach: () => {
		setupChatSpies({
			providerConfigs: [
				createProviderConfig({
					id: "provider-bedrock-invalid",
					provider: "bedrock",
					display_name: "Bedrock",
					has_api_key: false,
					central_api_key_enabled: false,
					allow_user_api_key: false,
					allow_central_api_key_fallback: false,
				}),
			],
			modelConfigs: [],
			modelCatalog: { providers: [] },
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("button", { name: /Bedrock/i }),
		);
		await expect(
			body.findByText("At least one credential source must be enabled"),
		).resolves.toBeInTheDocument();
		expect(body.getByRole("button", { name: "Save changes" })).toBeDisabled();
	},
};

export const ProviderFormBedrockAmbientCredentials: Story = {
	args: {
		section: "providers" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: nilProviderConfigID,
				provider: "bedrock",
				display_name: "AWS Bedrock",
				source: "supported",
				enabled: false,
			}),
		],
		modelCatalogData: { providers: [] },
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("button", { name: /AWS Bedrock/i }),
		);

		const apiKeyInput = await body.findByLabelText(/^API Key$/i);
		const createButton = body.getByRole("button", {
			name: "Create provider config",
		});

		await expect(apiKeyInput).not.toBeRequired();
		await expect(apiKeyInput).toHaveAttribute(
			"placeholder",
			"Enter bearer token",
		);
		await expect(
			body.findByText(
				"Bearer token for Bedrock authentication. Leave empty to use ambient AWS credentials.",
			),
		).resolves.toBeInTheDocument();
		await expect(
			body.findByText(
				/Overrides the Bedrock runtime endpoint\.\s+Set AWS_REGION on\s+the Coder server to select the target region\./i,
			),
		).resolves.toBeInTheDocument();
		await expect(createButton).toBeEnabled();

		await userEvent.click(createButton);
		await waitFor(() => {
			expect(args.onCreateProvider).toHaveBeenCalledTimes(1);
		});
		const createProviderMock = args.onCreateProvider as ReturnType<typeof fn>;
		const createRequest = createProviderMock.mock.calls[0][0] as Record<
			string,
			unknown
		>;
		expect(createRequest).toMatchObject({
			provider: "bedrock",
			central_api_key_enabled: true,
			allow_user_api_key: false,
			allow_central_api_key_fallback: false,
		});
		expect(createRequest).not.toHaveProperty("api_key");
	},
};

export const ProviderFormBedrockBearerToken: Story = {
	args: {
		section: "providers" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-bedrock-bearer",
				provider: "bedrock",
				display_name: "AWS Bedrock",
				has_api_key: true,
				central_api_key_enabled: true,
				allow_user_api_key: false,
				allow_central_api_key_fallback: false,
			}),
		],
		modelCatalogData: { providers: [] },
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("button", { name: /AWS Bedrock/i }),
		);

		const apiKeyInput = await body.findByLabelText(/^API Key$/i);
		const saveButton = body.getByRole("button", { name: "Save changes" });

		await expect(apiKeyInput).not.toBeRequired();
		await expect(apiKeyInput).toHaveValue("••••••••••••••••");

		await userEvent.click(apiKeyInput);
		await userEvent.type(apiKeyInput, "bedrock-bearer-token");
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onUpdateProvider).toHaveBeenCalledTimes(1);
		});
		expect(args.onUpdateProvider).toHaveBeenCalledWith(
			"provider-bedrock-bearer",
			expect.objectContaining({ api_key: "bedrock-bearer-token" }),
		);
	},
};

export const ProviderFormBedrockClearBearerToken: Story = {
	args: {
		section: "providers" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-bedrock-clear",
				provider: "bedrock",
				display_name: "AWS Bedrock",
				has_api_key: true,
				central_api_key_enabled: true,
				allow_user_api_key: false,
				allow_central_api_key_fallback: false,
			}),
		],
		modelCatalogData: { providers: [] },
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("button", { name: /AWS Bedrock/i }),
		);

		const apiKeyInput = await body.findByLabelText(/^API Key$/i);
		const clearStoredTokenButton = body.getByRole("button", {
			name: /Clear stored token/i,
		});
		const saveButton = body.getByRole("button", { name: "Save changes" });

		await expect(apiKeyInput).toHaveValue("••••••••••••••••");
		await userEvent.click(clearStoredTokenButton);
		await waitFor(() => {
			expect(apiKeyInput).toHaveValue("");
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onUpdateProvider).toHaveBeenCalledTimes(1);
		});
		expect(args.onUpdateProvider).toHaveBeenCalledWith(
			"provider-bedrock-clear",
			expect.objectContaining({ api_key: "" }),
		);
	},
};

const openAddModelForm = async (
	body: ReturnType<typeof within>,
	providerLabel: string,
) => {
	// Click the dropdown trigger to open the provider menu.
	const trigger = await body.findByRole("button", { name: "Add model" });
	await userEvent.click(trigger);
	// Radix portals dropdown content into the document body.
	// Wait for the menu to appear and click the provider item.
	await waitFor(async () => {
		const item = body.getByRole("menuitem", {
			name: new RegExp(providerLabel, "i"),
		});
		await userEvent.click(item);
	});
};

/** Expand a collapsible section by clicking its header button. */
const expandSection = async (body: ReturnType<typeof within>, name: string) => {
	const btn = await body.findByRole("button", {
		name: new RegExp(name, "i"),
	});
	await userEvent.click(btn);
};

const enterModelIdentifier = async (
	body: ReturnType<typeof within>,
	value: string,
) => {
	const field = await body.findByLabelText(/Model Identifier/i);
	if (field instanceof HTMLInputElement) {
		await userEvent.type(field, value);
		return;
	}

	await userEvent.click(field);
	await userEvent.type(await body.findByRole("combobox"), value);
};

export const NoModelConfigByDefault: Story = {
	args: {
		section: "models" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-openai",
				provider: "openai",
				display_name: "OpenAI",
				source: "database",
				has_api_key: true,
			}),
		],
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Open "Add model" dropdown and select the OpenAI provider.
		await openAddModelForm(body, "OpenAI");

		await enterModelIdentifier(body, "gpt-5-pro");
		await userEvent.type(body.getByLabelText(/Context limit/i), "200000");

		// Max output tokens is under the "Advanced" toggle.
		await userEvent.click(body.getByText("Advanced"));
		await expect(await body.findByLabelText(/Max output tokens/i)).toHaveValue(
			"",
		);

		// The submit button in ModelForm also says "Add model".
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
		// Blank pricing fields should remain unset in the payload.
		const createModelMock = args.onCreateModel as ReturnType<typeof fn>;
		const callArgs = createModelMock.mock.calls[0][0] as Record<
			string,
			unknown
		>;
		expect(callArgs).not.toHaveProperty("model_config");
	},
};

export const SubmitModelConfigExplicitly: Story = {
	args: {
		section: "models" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-openai",
				provider: "openai",
				display_name: "OpenAI",
				source: "database",
				has_api_key: true,
			}),
		],
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Open "Add model" dropdown and select the OpenAI provider.
		await openAddModelForm(body, "OpenAI");

		await enterModelIdentifier(body, "gpt-5-pro-custom");
		await userEvent.type(body.getByLabelText(/Context limit/i), "200000");
		// Max output tokens is under "Advanced".
		await expandSection(body, "Advanced");
		await userEvent.type(
			await body.findByLabelText(/Max output tokens/i),
			"32000",
		);
		// Reasoning Effort is a provider option under "Provider Configuration".
		await expandSection(body, "Provider Configuration");
		const effortGroup = await body.findByRole("radiogroup", {
			name: "Reasoning Effort",
		});
		await userEvent.click(within(effortGroup).getByText("High"));

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
	args: {
		section: "models" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-openai",
				provider: "openai",
				display_name: "OpenAI",
				source: "database",
				has_api_key: true,
			}),
		],
		modelConfigsData: [
			createModelConfig({
				id: "model-enabled",
				provider: "openai",
				model: "gpt-test-enabled",
				display_name: "GPT Test Enabled",
				enabled: true,
			}),
		],
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
	},
};

// ── Per-provider model form stories ────────────────────────────
// Each story opens the "Add model" form for a specific provider
// so you can visually verify the schema-driven fields render.

const providerFormSetup = (provider: string, displayName: string) => ({
	args: {
		section: "models" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: `provider-${provider}`,
				provider,
				display_name: displayName,
				source: "database",
				has_api_key: true,
			}),
		],
	},
});

const findOptionByText = (options: HTMLElement[], text: string) => {
	for (const option of options) {
		if (option.textContent?.includes(text)) {
			return option;
		}
	}
	throw new Error(`Expected visible option containing ${text}.`);
};

const expectKnownModelOptionsInOrder = async (
	body: ReturnType<typeof within>,
	provider: string,
) => {
	const knownModels = getKnownModelsForProvider(provider);
	const options = await body.findAllByRole("option");
	expect(options.length).toBeGreaterThanOrEqual(knownModels.length);

	for (const [index, knownModel] of knownModels.entries()) {
		const option = options[index];
		if (!option) {
			throw new Error(`Expected option at index ${index}.`);
		}
		expect(option).toHaveTextContent(knownModel.displayName);
		expect(option).toHaveTextContent(knownModel.modelIdentifier);
		if (knownModel.contextLimit !== undefined) {
			expect(option).toHaveTextContent(
				formatContextBadge(knownModel.contextLimit),
			);
		}
	}

	return options;
};

const knownModelDefaultsFeedback = (displayName: string) =>
	`Defaults applied from ${displayName}. Review and adjust before saving.`;

const noMatchingKnownModelsText =
	"No matching known models. You can still use this identifier.";

const openKnownModelPopover = async (body: ReturnType<typeof within>) => {
	await userEvent.click(await body.findByLabelText(/Model Identifier/i));
	const input = await body.findByRole("combobox");
	await expect(input).toHaveFocus();
	return input;
};

const expectKnownModelPopoverClosed = async (
	body: ReturnType<typeof within>,
) => {
	await waitFor(() => {
		expect(body.queryByRole("listbox")).not.toBeInTheDocument();
		expect(body.queryAllByRole("option")).toHaveLength(0);
		expect(body.queryByText(noMatchingKnownModelsText)).not.toBeInTheDocument();
	});
};

const closeKnownModelPopoverToContextLimit = async (
	body: ReturnType<typeof within>,
) => {
	await userEvent.click(body.getByLabelText(/Context limit/i));
	await expectKnownModelPopoverClosed(body);
};

const selectKnownModel = async (
	body: ReturnType<typeof within>,
	modelIdentifier: string,
) => {
	const input = await openKnownModelPopover(body);
	await userEvent.clear(input);
	await expect(input).toHaveValue("");
	const options = await body.findAllByRole("option");
	await userEvent.click(findOptionByText(options, modelIdentifier));
	await expectModelIdentifierValue(body, modelIdentifier);
};

const clearAndTypeKnownModelSearch = async (
	body: ReturnType<typeof within>,
	value: string,
) => {
	let input = await body.findByRole("combobox");
	await userEvent.clear(input);
	input = await body.findByRole("combobox");
	await expect(input).toHaveValue("");
	await expect(input).toHaveFocus();
	await userEvent.keyboard(value);
	input = await body.findByRole("combobox");
	await expect(input).toHaveValue(value);
	return input;
};

const expectModelIdentifierValue = async (
	body: ReturnType<typeof within>,
	value: string,
) => {
	const control = await body.findByLabelText(/Model Identifier/i);
	if (control.matches("input,textarea")) {
		await waitFor(() => expect(control).toHaveValue(value));
		return;
	}

	await waitFor(() => expect(control).toHaveTextContent(value));
};

const getDefaultsFeedback = (
	body: ReturnType<typeof within>,
	message: string,
) =>
	body
		.queryAllByRole("status")
		.filter((el: HTMLElement) => el.textContent === message);

const expectDefaultsFeedbackCount = (
	body: ReturnType<typeof within>,
	message: string,
	count: number,
) => {
	expect(getDefaultsFeedback(body, message)).toHaveLength(count);
};

const expectOffCatalogModelCommitted = async (
	body: ReturnType<typeof within>,
	value: string,
) => {
	await openKnownModelPopover(body);
	await clearAndTypeKnownModelSearch(body, value);
	await closeKnownModelPopoverToContextLimit(body);

	await expectModelIdentifierValue(body, value);
	expect(body.queryByRole("status")).not.toBeInTheDocument();
	expect(body.queryByText("Model ID is required.")).not.toBeInTheDocument();
};

const ensureCostTrackingOpen = async (body: ReturnType<typeof within>) => {
	if (body.queryByLabelText(/^Input$/i)) {
		return;
	}
	await expandSection(body, "Cost Tracking");
	await body.findByLabelText(/^Input$/i);
};

const expectPricingValue = async (
	body: ReturnType<typeof within>,
	label: RegExp,
	value: string,
) => {
	await expect(await body.findByLabelText(label)).toHaveValue(value);
};

const expectReasoningEffort = async (
	body: ReturnType<typeof within>,
	value: string,
) => {
	const reasoningEffortGroup = await body.findByRole("radiogroup", {
		name: "Reasoning Effort",
	});

	if (value === "") {
		for (const option of within(reasoningEffortGroup).getAllByRole("radio")) {
			await expect(option).toHaveAttribute("aria-checked", "false");
		}
		return;
	}

	const label = value.charAt(0).toUpperCase() + value.slice(1);
	await expect(
		within(reasoningEffortGroup).getByRole("radio", { name: label }),
	).toHaveAttribute("aria-checked", "true");
};

type OpenAIDefaultExpectations = {
	modelIdentifier: string;
	contextLimit: string;
	maxCompletionTokens: string;
	reasoningEffort: string;
	inputCost: string;
	outputCost: string;
	cacheReadCost?: string;
	cacheWriteCost?: string;
};

const gpt55Defaults = {
	modelIdentifier: "gpt-5.5",
	contextLimit: "1050000",
	maxCompletionTokens: "128000",
	reasoningEffort: "medium",
	inputCost: "5",
	outputCost: "30",
	cacheReadCost: "0.5",
} satisfies OpenAIDefaultExpectations;

const gpt55ProDefaults = {
	modelIdentifier: "gpt-5.5-pro",
	contextLimit: "1050000",
	maxCompletionTokens: "128000",
	reasoningEffort: "high",
	inputCost: "30",
	outputCost: "180",
} satisfies OpenAIDefaultExpectations;

const gpt54MiniDefaults = {
	modelIdentifier: "gpt-5.4-mini",
	contextLimit: "400000",
	maxCompletionTokens: "128000",
	reasoningEffort: "medium",
	inputCost: "0.75",
	outputCost: "4.5",
	cacheReadCost: "0.075",
} satisfies OpenAIDefaultExpectations;

const ensureProviderConfigurationOpen = async (
	body: ReturnType<typeof within>,
) => {
	if (body.queryByLabelText(/Max Completion Tokens/i)) {
		return;
	}
	await expandSection(body, "Provider Configuration");
	await body.findByLabelText(/Max Completion Tokens/i);
};

const expectOpenAIKnownModelDefaults = async (
	body: ReturnType<typeof within>,
	expectations: OpenAIDefaultExpectations,
) => {
	await expectModelIdentifierValue(body, expectations.modelIdentifier);
	await expect(body.getByLabelText(/Context limit/i)).toHaveValue(
		expectations.contextLimit,
	);

	await ensureProviderConfigurationOpen(body);
	await expect(
		await body.findByLabelText(/Max Completion Tokens/i),
	).toHaveValue(expectations.maxCompletionTokens);
	await expectReasoningEffort(body, expectations.reasoningEffort);

	await ensureCostTrackingOpen(body);
	await expectPricingValue(body, /^Input$/i, expectations.inputCost);
	await expectPricingValue(body, /^Output$/i, expectations.outputCost);
	await expectPricingValue(
		body,
		/^Cache Read$/i,
		expectations.cacheReadCost ?? "",
	);
	await expectPricingValue(
		body,
		/^Cache Write$/i,
		expectations.cacheWriteCost ?? "",
	);
};

export const OpenAIKnownModelHappyPath: Story = {
	...providerFormSetup("openai", "OpenAI"),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");

		await openKnownModelPopover(body);
		const options = await expectKnownModelOptionsInOrder(body, "openai");
		await userEvent.click(findOptionByText(options, "gpt-5.5"));

		await expectModelIdentifierValue(body, "gpt-5.5");
		await expect(await body.findByRole("status")).toHaveTextContent(
			"Defaults applied from GPT-5.5. Review and adjust before saving.",
		);
		await expect(body.getByLabelText(/Context limit/i)).toHaveValue("1050000");

		await expandSection(body, "Provider Configuration");
		await expect(
			await body.findByLabelText(/Max Completion Tokens/i),
		).toHaveValue("128000");
	},
};

export const OpenAIKnownModelKeyboardSelection: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-25: keyboard selection applies defaults",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");

		const input = await openKnownModelPopover(body);
		fireEvent.keyDown(input, { key: "ArrowDown" });
		await userEvent.keyboard("{Enter}");

		await expectOpenAIKnownModelDefaults(body, gpt55ProDefaults);
		await expect(await body.findByRole("status")).toHaveTextContent(
			knownModelDefaultsFeedback("GPT-5.5 Pro"),
		);
	},
};

export const OpenAIKnownModelReclickSelectedDoesNotClearField: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-26: re-clicking selected Known Model does not clear field",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");

		await selectKnownModel(body, "gpt-5.5");
		await openKnownModelPopover(body);
		const options = await body.findAllByRole("option");
		await userEvent.click(findOptionByText(options, "gpt-5.5"));

		await expectModelIdentifierValue(body, "gpt-5.5");
		await expectKnownModelPopoverClosed(body);
		expect(
			body.queryByRole("button", { name: /clear/i }),
		).not.toBeInTheDocument();
	},
};

export const AnthropicKnownModelHappyPath: Story = {
	...providerFormSetup("anthropic", "Anthropic"),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "Anthropic");

		await openKnownModelPopover(body);
		const options = await body.findAllByRole("option");
		await userEvent.click(findOptionByText(options, "claude-opus-4-7"));

		await expectModelIdentifierValue(body, "claude-opus-4-7");
		await expect(body.getByLabelText(/Context limit/i)).toHaveValue("1000000");

		await expandSection(body, "Advanced");
		await expect(await body.findByLabelText(/Max Output Tokens/i)).toHaveValue(
			"128000",
		);

		await expandSection(body, "Provider Configuration");
		const sendReasoningGroup = await body.findByRole("radiogroup", {
			name: "Send Reasoning",
		});
		await expect(
			within(sendReasoningGroup).getByRole("radio", { name: "On" }),
		).toHaveAttribute("aria-checked", "false");
		await expect(
			within(sendReasoningGroup).getByRole("radio", { name: "Off" }),
		).toHaveAttribute("aria-checked", "false");
		await expect(
			await body.findByLabelText(/Thinking Budget Tokens/i),
		).toHaveValue("");
		await expectReasoningEffort(body, "high");
	},
};

export const AnthropicHaikuKnownModelUsesThinkingBudgetNotEffort: Story = {
	...providerFormSetup("anthropic", "Anthropic"),
	name: "Add mode / DEREM-43: Haiku 4.5 sets thinking budget instead of effort",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "Anthropic");
		await selectKnownModel(body, "claude-haiku-4-5");

		await expandSection(body, "Provider Configuration");

		// Reasoning Effort should remain empty because Haiku 4.5 uses the
		// thinking budget path instead of Anthropic adaptive thinking.
		await expectReasoningEffort(body, "");
		await expect(
			await body.findByLabelText(/Thinking Budget Tokens/i),
		).toHaveValue("8192");
	},
};

export const OpenAIKnownModelDoesNotPreFireRequiredError: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-3: open does not pre-fire required error",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");

		await openKnownModelPopover(body);

		expect(body.queryByText("Model ID is required.")).not.toBeInTheDocument();
	},
};

export const OpenAIKnownModelOpenDoesNotFlashInvalidBorder: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-31: open does not flash invalid border on trigger",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");

		const trigger = await body.findByLabelText(/Model Identifier/i);
		await openKnownModelPopover(body);

		expect([null, "false"]).toContain(trigger.getAttribute("aria-invalid"));
		expect(trigger).not.toHaveClass("border-content-destructive");
		expect(body.queryByText("Model ID is required.")).not.toBeInTheDocument();
	},
};

export const KnownModelClickOffEmptyDoesNotFireRequired: Story = {
	...providerFormSetup("anthropic", "Anthropic"),
	name: "Add mode / DEREM-47: clicking off empty model does not fire required error",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "Anthropic");

		const trigger = await body.findByLabelText(/Model Identifier/i);
		await openKnownModelPopover(body);

		// Click another field to close the popover without typing or selecting.
		// Mirrors the QA-reported flow: focus the field, change your mind, click
		// elsewhere; the empty value should NOT surface "Model ID is required."
		// before the user has actually attempted to commit anything.
		await closeKnownModelPopoverToContextLimit(body);

		expect(body.queryByText("Model ID is required.")).not.toBeInTheDocument();
		expect([null, "false"]).toContain(trigger.getAttribute("aria-invalid"));
		expect(trigger).not.toHaveClass("border-content-destructive");
	},
};

export const OpenAIKnownModelEscapeCancelsSearch: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-5: Escape cancels and preserves committed value",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const feedback = knownModelDefaultsFeedback("GPT-5.5");
		await openAddModelForm(body, "OpenAI");

		await selectKnownModel(body, "gpt-5.5");
		await expectModelIdentifierValue(body, "gpt-5.5");
		expectDefaultsFeedbackCount(body, feedback, 1);

		await openKnownModelPopover(body);
		await clearAndTypeKnownModelSearch(body, "cod");
		await userEvent.keyboard("{Escape}");

		await expectKnownModelPopoverClosed(body);
		await expectModelIdentifierValue(body, "gpt-5.5");
		expectDefaultsFeedbackCount(body, feedback, 1);

		const reopenedInput = await openKnownModelPopover(body);
		await expect(reopenedInput).toHaveValue("gpt-5.5");
		await userEvent.keyboard("{Escape}");
	},
};

export const OpenAIKnownModelEscapeDoesNotReapplyDefaultsFeedback: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-30: type-then-Escape does not re-apply defaults feedback",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const feedback = knownModelDefaultsFeedback("GPT-5.5");
		await openAddModelForm(body, "OpenAI");

		await selectKnownModel(body, "gpt-5.5");
		expectDefaultsFeedbackCount(body, feedback, 1);
		const initialFeedback = getDefaultsFeedback(body, feedback)[0];
		if (!initialFeedback) {
			throw new Error("Expected Known Model defaults feedback.");
		}

		await openKnownModelPopover(body);
		await clearAndTypeKnownModelSearch(body, "a");
		await userEvent.keyboard("{Escape}");

		await expectKnownModelPopoverClosed(body);
		await userEvent.click(body.getByLabelText(/Context limit/i));

		expectDefaultsFeedbackCount(body, feedback, 1);
		expect(getDefaultsFeedback(body, feedback)[0]).toBe(initialFeedback);
		await expectModelIdentifierValue(body, "gpt-5.5");
		await expect(body.getByLabelText(/Context limit/i)).toHaveValue("1050000");
	},
};

export const OpenAIKnownModelSequentialSelectionReplacesDefaults: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-4, DEREM-10: sequential selection replaces catalog defaults",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");

		await selectKnownModel(body, "gpt-5.5");
		await selectKnownModel(body, "gpt-5.4-mini");

		await expectModelIdentifierValue(body, "gpt-5.4-mini");
		await expect(body.getByLabelText(/Context limit/i)).toHaveValue("400000");
		await ensureCostTrackingOpen(body);
		await expectPricingValue(body, /^Input$/i, "0.75");
		await expectPricingValue(body, /^Output$/i, "4.5");
	},
};

export const OpenAIKnownModelReasoningEffortClearsForNonReasoningModel: Story =
	{
		...providerFormSetup("openai", "OpenAI"),
		name: "Add mode / DEREM-31: reasoningEffort clears when switching to non-reasoning model",
		play: async ({ canvasElement }) => {
			const body = within(canvasElement.ownerDocument.body);
			await openAddModelForm(body, "OpenAI");

			await selectKnownModel(body, "gpt-5.5");
			await ensureProviderConfigurationOpen(body);
			await expectReasoningEffort(body, "medium");

			await selectKnownModel(body, "gpt-5.4");
			await expectReasoningEffort(body, "");
		},
	};

export const OpenAIKnownModelStaleCostFieldDoesNotPersist: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-24: stale cost fields do not persist",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");

		await selectKnownModel(body, "gpt-5.5-pro");
		await selectKnownModel(body, "gpt-5.4-mini");
		await selectKnownModel(body, "gpt-5.5");

		await expectOpenAIKnownModelDefaults(body, gpt55Defaults);
	},
};

export const OpenAIKnownModelOffCatalogInterleavingKeepsTracking: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-24: off-catalog interleaving keeps tracking",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");

		await selectKnownModel(body, "gpt-5.5");
		await openKnownModelPopover(body);
		await clearAndTypeKnownModelSearch(body, "my-custom-fine-tune");
		await closeKnownModelPopoverToContextLimit(body);
		await expectModelIdentifierValue(body, "my-custom-fine-tune");

		await selectKnownModel(body, "gpt-5.4-mini");

		await expectOpenAIKnownModelDefaults(body, gpt54MiniDefaults);
	},
};

export const OpenAIKnownModelChainTrackingDoesNotLoseFields: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-24: chained selections retain tracking",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");

		await selectKnownModel(body, "gpt-5.5");
		await selectKnownModel(body, "gpt-5.5-pro");
		await expectOpenAIKnownModelDefaults(body, gpt55ProDefaults);
		await selectKnownModel(body, "gpt-5.4-mini");

		await expectOpenAIKnownModelDefaults(body, gpt54MiniDefaults);
	},
};

export const OpenAIKnownModelDoubleApplyGuard: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-10: double-apply guard keeps defaults stable",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const feedback = knownModelDefaultsFeedback("GPT-5.5");
		await openAddModelForm(body, "OpenAI");

		await selectKnownModel(body, "gpt-5.5");
		await expect(await body.findByRole("status")).toHaveTextContent(feedback);
		await ensureCostTrackingOpen(body);
		await expect(body.getByLabelText(/Context limit/i)).toHaveValue("1050000");
		await expectPricingValue(body, /^Input$/i, "5");
		await expectPricingValue(body, /^Output$/i, "30");

		await openKnownModelPopover(body);
		await closeKnownModelPopoverToContextLimit(body);

		expectDefaultsFeedbackCount(body, feedback, 1);
		await expectModelIdentifierValue(body, "gpt-5.5");
		await expect(body.getByLabelText(/Context limit/i)).toHaveValue("1050000");
		await expectPricingValue(body, /^Input$/i, "5");
		await expectPricingValue(body, /^Output$/i, "30");
	},
};

export const OpenAIKnownModelExactCanonicalBlurAppliesDefaults: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-10: exact canonical blur applies defaults",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");

		await openKnownModelPopover(body);
		await clearAndTypeKnownModelSearch(body, "gpt-5.5-pro");
		await closeKnownModelPopoverToContextLimit(body);

		await expectModelIdentifierValue(body, "gpt-5.5-pro");
		await expect(body.getByLabelText(/Context limit/i)).toHaveValue("1050000");
		await ensureCostTrackingOpen(body);
		await expectPricingValue(body, /^Input$/i, "30");
		await expectPricingValue(body, /^Output$/i, "180");
		await expect(await body.findByRole("status")).toHaveTextContent(
			knownModelDefaultsFeedback("GPT-5.5 Pro"),
		);
	},
};

export const AnthropicKnownModelAliasTypedValueCancels: Story = {
	...providerFormSetup("anthropic", "Anthropic"),
	name: "Add mode / DEREM-10: alias typed value cancels",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "Anthropic");

		await openKnownModelPopover(body);
		await clearAndTypeKnownModelSearch(body, "custom-anthropic-model");
		await closeKnownModelPopoverToContextLimit(body);
		await expectModelIdentifierValue(body, "custom-anthropic-model");

		await openKnownModelPopover(body);
		await clearAndTypeKnownModelSearch(body, "claude-haiku-4-5-20251001");
		const filteredOptions = await body.findAllByRole("option");
		expect(
			findOptionByText(filteredOptions, "Claude Haiku 4.5"),
		).toHaveTextContent("claude-haiku-4-5");
		await closeKnownModelPopoverToContextLimit(body);

		await expectModelIdentifierValue(body, "custom-anthropic-model");
		expect(body.queryByRole("status")).not.toBeInTheDocument();
		await expect(body.getByLabelText(/Context limit/i)).toHaveValue("");
	},
};

export const AnthropicKnownModelPunctuationVariantCommits: Story = {
	...providerFormSetup("anthropic", "Anthropic"),
	name: "Add mode / DEREM-27: off-catalog with punctuation variant commits",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "Anthropic");

		await expectOffCatalogModelCommitted(body, "claude.haiku.4.5.20251001");
	},
};

export const KnownModelOffCatalogSubstringCommits: Story = {
	args: {
		section: "models" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-anthropic-known-model-substring",
				provider: "anthropic",
				display_name: "Anthropic",
				source: "database",
				has_api_key: true,
			}),
			createProviderConfig({
				id: "provider-openai-known-model-substring",
				provider: "openai",
				display_name: "OpenAI",
				source: "database",
				has_api_key: true,
			}),
		],
	},
	name: "Add mode / DEREM-19: off-catalog identifier substring-matching catalog metadata commits",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		await openAddModelForm(body, "Anthropic");
		await expectOffCatalogModelCommitted(body, "haiku");
		await userEvent.click(body.getByRole("button", { name: /^Cancel$/i }));
		await waitFor(() => {
			expect(
				body.queryByLabelText(/Model Identifier/i),
			).not.toBeInTheDocument();
		});

		await openAddModelForm(body, "OpenAI");
		await expectOffCatalogModelCommitted(body, "mini");
		await expectOffCatalogModelCommitted(body, "pro");
		await expectOffCatalogModelCommitted(body, "gpt-5");
	},
};

export const KnownModelProviderChangeResetsDefaultsFeedback: Story = {
	args: {
		section: "models" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-openai-known-model-reset",
				provider: "openai",
				display_name: "OpenAI",
				source: "database",
				has_api_key: true,
			}),
			createProviderConfig({
				id: "provider-anthropic-known-model-reset",
				provider: "anthropic",
				display_name: "Anthropic",
				source: "database",
				has_api_key: true,
			}),
		],
	},
	name: "Add mode / DEREM-10: provider change resets Known Model defaults",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");

		await selectKnownModel(body, "gpt-5.5");
		await expect(await body.findByRole("status")).toHaveTextContent(
			knownModelDefaultsFeedback("GPT-5.5"),
		);

		await userEvent.click(body.getByRole("button", { name: /^Cancel$/i }));
		await waitFor(() => {
			expect(
				body.queryByLabelText(/Model Identifier/i),
			).not.toBeInTheDocument();
		});
		await openAddModelForm(body, "Anthropic");
		await selectKnownModel(body, "claude-haiku-4-5");

		await expectModelIdentifierValue(body, "claude-haiku-4-5");
		await expect(body.getByLabelText(/Context limit/i)).toHaveValue("200000");
		await ensureCostTrackingOpen(body);
		await expectPricingValue(body, /^Input$/i, "1");
		await expectPricingValue(body, /^Output$/i, "5");
		await expect(await body.findByRole("status")).toHaveTextContent(
			knownModelDefaultsFeedback("Claude Haiku 4.5"),
		);
	},
};

export const OpenAIKnownModelTriggerAriaParity: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-1: aria parity on autocomplete trigger",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");

		// Surface the required-field error through a real user action:
		// open the popover, type then clear the search, and click off. The
		// off-catalog close path commits an empty string and marks the field
		// touched, so Formik validation renders "Model ID is required." This
		// verifies the inline-search trigger forwards aria-invalid +
		// aria-describedby with the same parity as the plain <Input>
		// fallback used in edit/duplicate modes.
		await openKnownModelPopover(body);
		const input = await clearAndTypeKnownModelSearch(body, "x");
		await userEvent.clear(input);
		await closeKnownModelPopoverToContextLimit(body);

		const trigger = await body.findByLabelText(/Model Identifier/i);
		const error = await body.findByText("Model ID is required.");
		expect(error.id).toBeTruthy();
		await expect(trigger).toHaveAttribute("aria-invalid", "true");
		await expect(trigger).toHaveAttribute("aria-describedby", error.id);
	},
};

export const OpenAIKnownModelNoOptionsCopy: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-6: no-options auto-hides popover",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");

		await openKnownModelPopover(body);
		await clearAndTypeKnownModelSearch(body, "zzzzzzz");

		await expectKnownModelPopoverClosed(body);
		expect(body.queryByText(noMatchingKnownModelsText)).not.toBeInTheDocument();
	},
};

export const AnthropicKnownModelEnterCommitsOffCatalogIdentifier: Story = {
	...providerFormSetup("anthropic", "Anthropic"),
	name: "Add mode / DEREM-34: Enter commits off-catalog identifier",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "Anthropic");

		const input = await openKnownModelPopover(body);
		await userEvent.type(input, "claude-opus-4-5");
		await expectKnownModelPopoverClosed(body);
		await expect(input).toHaveAttribute("aria-expanded", "false");
		await userEvent.keyboard("{Enter}");

		await expectKnownModelPopoverClosed(body);
		await expectModelIdentifierValue(body, "claude-opus-4-5");
		expect(body.queryByRole("status")).not.toBeInTheDocument();
	},
};

export const KnownModelAutoHidePopoverWhenNoMatches: Story = {
	...providerFormSetup("anthropic", "Anthropic"),
	name: "Add mode / DEREM-42: popover auto-hides when search has no matches",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "Anthropic");

		const input = await body.findByRole("combobox", {
			name: /Model Identifier/i,
		});
		await userEvent.click(input);
		await expect(await body.findByText("Claude Opus 4.7")).toBeInTheDocument();

		await userEvent.clear(input);
		await userEvent.type(input, "claude-opus-4-5");

		await waitFor(() => {
			expect(body.queryByRole("listbox")).not.toBeInTheDocument();
		});
		expect(
			body.queryByText(/No matching known models/i),
		).not.toBeInTheDocument();
		await expect(input).toHaveAttribute("aria-expanded", "false");

		await userEvent.keyboard("{Enter}");
		await expectModelIdentifierValue(body, "claude-opus-4-5");
	},
};

export const KnownModelBlurAfterAutoHideCommitsOffCatalog: Story = {
	...providerFormSetup("anthropic", "Anthropic"),
	name: "Add mode / DEREM-45: blur after auto-hide commits off-catalog identifier",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "Anthropic");

		const input = await body.findByRole("combobox", {
			name: /Model Identifier/i,
		});
		await userEvent.click(input);
		await userEvent.clear(input);
		await userEvent.type(input, "claude-opus-4-5");

		// Popover should auto-hide for the unmatched query.
		await waitFor(() => {
			expect(body.queryByRole("listbox")).not.toBeInTheDocument();
		});
		await expect(input).toHaveAttribute("aria-expanded", "false");

		// Blur via Tab: focus moves to the next field, exercising the
		// handleBlur auto-hide path that calls handleOpenChange(false).
		await userEvent.tab();

		await expectModelIdentifierValue(body, "claude-opus-4-5");
		// No defaults feedback for off-catalog identifiers.
		expect(body.queryByRole("status")).not.toBeInTheDocument();
		// Critical: in the buggy variant where handleBlur skips the
		// handleOpenChange(false) branch for the auto-hidden popover,
		// the inline input still visually shows the typed search text
		// (via the controlled inputValue prop), so any DOM-value
		// assertion would pass vacuously. The committed form value is
		// what diverges, surfaced here via the required-field error:
		// markTouched() runs in the buggy path with form.values.model
		// still empty, producing "Model ID is required." The fixed path
		// commits the typed text via setFieldValue first, clearing the
		// validation error.
		expect(body.queryByText("Model ID is required.")).not.toBeInTheDocument();
	},
};

export const OpenAIKnownModelTriggerInputIsTypedField: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-35: trigger input is the typed field",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");

		const input = await openKnownModelPopover(body);
		await userEvent.type(input, "5.4");

		await expect(input).toHaveFocus();
		await expect(input).toHaveValue("5.4");
		const options = await body.findAllByRole("option");
		expect(findOptionByText(options, "gpt-5.4")).toBeInTheDocument();
		expect(findOptionByText(options, "gpt-5.4-mini")).toBeInTheDocument();
		expect(findOptionByText(options, "gpt-5.4-nano")).toBeInTheDocument();
		expect(body.queryByText("gpt-5.5")).not.toBeInTheDocument();
		expect(body.queryByText("gpt-5.5-pro")).not.toBeInTheDocument();
		expect(body.queryByText("gpt-5.3-codex")).not.toBeInTheDocument();
	},
};

export const OpenAIKnownModelArrowDownEnterSelectsHighlighted: Story = {
	...providerFormSetup("openai", "OpenAI"),
	name: "Add mode / DEREM-36: ArrowDown Enter selects highlighted option",
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");

		const input = await openKnownModelPopover(body);
		fireEvent.keyDown(input, { key: "ArrowDown" });
		await userEvent.keyboard("{Enter}");

		await expectModelIdentifierValue(body, "gpt-5.5-pro");
		await expectOpenAIKnownModelDefaults(body, gpt55ProDefaults);
		await expect(await body.findByRole("status")).toHaveTextContent(
			knownModelDefaultsFeedback("GPT-5.5 Pro"),
		);
	},
};

export const UnsupportedProviderFallback: Story = {
	...providerFormSetup("google", "Google"),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "Google");

		const modelInput = await body.findByLabelText(/Model Identifier/i);
		await userEvent.click(modelInput);
		expect(body.queryByRole("option")).not.toBeInTheDocument();
		await userEvent.type(modelInput, "gemini-custom-model");
		await userEvent.tab();

		await expect(modelInput).toHaveValue("gemini-custom-model");
		expect(body.queryByText("Model ID is required.")).not.toBeInTheDocument();
		expect(body.queryByRole("status")).not.toBeInTheDocument();
	},
};

export const ModelFormOpenAI: Story = {
	...providerFormSetup("openai", "OpenAI"),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await openAddModelForm(body, "OpenAI");
		await expandSection(body, "Provider Configuration");
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
		await expandSection(body, "Provider Configuration");
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
		await expandSection(body, "Provider Configuration");
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
		await expandSection(body, "Provider Configuration");
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
		await expandSection(body, "Provider Configuration");
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
		await expandSection(body, "Provider Configuration");
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
		await expandSection(body, "Provider Configuration");
		// Azure aliases to OpenAI fields.
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
		await expandSection(body, "Provider Configuration");
		// Bedrock aliases to Anthropic fields.
		await expect(
			await body.findByLabelText(/Send Reasoning/i),
		).toBeInTheDocument();
		await expect(await body.findByLabelText(/Effort/i)).toBeInTheDocument();
	},
};

export const ModelPricingWarningInList: Story = {
	args: {
		section: "models" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-openai",
				provider: "openai",
				display_name: "OpenAI",
				source: "database",
				has_api_key: true,
			}),
		],
		modelConfigsData: [
			createModelConfig({
				id: "model-warning",
				provider: "openai",
				model: "gpt-4.1",
				display_name: "GPT-4.1",
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await expect(await body.findByText("GPT-4.1")).toBeInTheDocument();
		await expect(
			body.getByText("Model pricing is not defined"),
		).toBeInTheDocument();
	},
};

export const ModelDeleteConfirmation: Story = {
	args: {
		section: "models" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-openai",
				provider: "openai",
				display_name: "OpenAI",
				source: "database",
				has_api_key: true,
			}),
		],
		modelConfigsData: [
			createModelConfig({
				id: "model-1",
				provider: "openai",
				model: "gpt-4o",
				display_name: "GPT-4o",
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Click the model row to open the edit form.
		await userEvent.click(await body.findByText("GPT-4o"));

		// The Delete button should be visible in the footer.
		const deleteButton = await body.findByRole("button", { name: "Delete" });
		await expect(deleteButton).toBeInTheDocument();

		// Click Delete to show the confirmation dialog.
		await userEvent.click(deleteButton);

		// The confirmation dialog should appear - leave it visible
		// so the Chromatic snapshot captures this state.
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
	args: {
		section: "models" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-openai",
				provider: "openai",
				display_name: "OpenAI",
				source: "database",
				has_api_key: true,
			}),
		],
		modelConfigsData: [
			createModelConfig({
				id: "model-1",
				provider: "openai",
				model: "gpt-4o",
				display_name: "GPT-4o",
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Navigate to edit form, open delete dialog, then cancel.
		await userEvent.click(await body.findByText("GPT-4o"));
		await userEvent.click(await body.findByRole("button", { name: "Delete" }));
		await body.findByText(/Are you sure/i);
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		// The dialog should be closed and the form footer restored.
		await waitFor(() => {
			expect(body.queryByRole("dialog")).not.toBeInTheDocument();
		});
		await expect(
			body.findByRole("button", { name: "Delete" }),
		).resolves.toBeInTheDocument();
		expect(body.getByRole("button", { name: "Save" })).toBeInTheDocument();
	},
};

export const ModelDeleteConfirmed: Story = {
	args: {
		section: "models" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-openai",
				provider: "openai",
				display_name: "OpenAI",
				source: "database",
				has_api_key: true,
			}),
		],
		modelConfigsData: [
			createModelConfig({
				id: "model-1",
				provider: "openai",
				model: "gpt-4o",
				display_name: "GPT-4o",
			}),
		],
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Navigate to edit form, open delete dialog, then confirm.
		await userEvent.click(await body.findByText("GPT-4o"));
		await userEvent.click(await body.findByRole("button", { name: "Delete" }));
		await userEvent.click(
			await body.findByRole("button", { name: "Delete model" }),
		);

		// The delete callback should have been called.
		await waitFor(() => {
			expect(args.onDeleteModel).toHaveBeenCalledTimes(1);
		});
		expect(args.onDeleteModel).toHaveBeenCalledWith("model-1");
	},
};

export const ProviderDeleteConfirmation: Story = {
	args: {
		section: "providers" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-openai",
				provider: "openai",
				display_name: "OpenAI",
				source: "database",
				has_api_key: true,
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Navigate to the provider detail view.
		await userEvent.click(await body.findByRole("button", { name: /OpenAI/i }));

		// Click Delete to show the confirmation dialog.
		const deleteButton = await body.findByRole("button", { name: "Delete" });
		await userEvent.click(deleteButton);

		// The confirmation dialog should appear - leave it visible
		// so the Chromatic snapshot captures this state.
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
	args: {
		section: "providers" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-openai",
				provider: "openai",
				display_name: "OpenAI",
				source: "database",
				has_api_key: true,
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Navigate to provider detail, open delete dialog, then cancel.
		await userEvent.click(await body.findByRole("button", { name: /OpenAI/i }));
		await userEvent.click(await body.findByRole("button", { name: "Delete" }));
		await body.findByText(/Are you sure/i);
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		// The dialog should be closed and the form footer restored.
		await waitFor(() => {
			expect(body.queryByRole("dialog")).not.toBeInTheDocument();
		});
		await expect(
			body.findByRole("button", { name: "Delete" }),
		).resolves.toBeInTheDocument();
		expect(
			body.getByRole("button", { name: /Save changes/i }),
		).toBeInTheDocument();
	},
};

export const ProviderDeleteConfirmed: Story = {
	args: {
		section: "providers" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-openai",
				provider: "openai",
				display_name: "OpenAI",
				source: "database",
				has_api_key: true,
			}),
		],
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Navigate to provider detail, open delete dialog, then confirm.
		await userEvent.click(await body.findByRole("button", { name: /OpenAI/i }));
		await userEvent.click(await body.findByRole("button", { name: "Delete" }));
		await userEvent.click(
			await body.findByRole("button", { name: "Delete provider" }),
		);

		// The delete callback should have been called.
		await waitFor(() => {
			expect(args.onDeleteProvider).toHaveBeenCalledTimes(1);
		});
		expect(args.onDeleteProvider).toHaveBeenCalledWith("provider-openai");
	},
};

export const ValidatesModelConfigFields: Story = {
	args: {
		section: "models" as ChatModelAdminSection,
		providerConfigsData: [
			createProviderConfig({
				id: "provider-openai",
				provider: "openai",
				display_name: "OpenAI",
				source: "database",
				has_api_key: true,
			}),
		],
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Open "Add model" dropdown and select the OpenAI provider.
		await openAddModelForm(body, "OpenAI");

		await enterModelIdentifier(body, "gpt-5-pro");
		await userEvent.type(body.getByLabelText(/Context limit/i), "200000");
		// Max output tokens is under the "Advanced" toggle.
		await userEvent.click(body.getByText("Advanced"));
		const maxOutputTokensInput =
			await body.findByLabelText(/Max output tokens/i);
		await userEvent.type(maxOutputTokensInput, "not-a-number");
		await waitFor(() => {
			expect(body.getByRole("button", { name: "Add model" })).toBeDisabled();
		});
		// No callback should have been invoked.
		expect(args.onCreateModel).not.toHaveBeenCalled();
	},
};
