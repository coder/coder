import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ComponentProps, useState } from "react";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	ChatModelAdminPanel,
	type ChatModelAdminSection,
} from "./ChatModelAdminPanel";

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
			body.getByRole("switch", { name: "Central API key" }),
		).not.toBeChecked();
		await expect(
			body.getByRole("switch", { name: "Allow user API keys" }),
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

		await userEvent.type(body.getByLabelText(/Model Identifier/i), "gpt-5-pro");
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

		await userEvent.type(
			body.getByLabelText(/Model Identifier/i),
			"gpt-5-pro-custom",
		);
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

		await userEvent.type(body.getByLabelText(/Model Identifier/i), "gpt-5-pro");
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
