import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
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
	created_at: overrides.created_at ?? now,
	updated_at: overrides.updated_at ?? now,
});

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
	spyOn(API, "getChatProviderConfigs").mockImplementation(async () => {
		return state.providerConfigs;
	});
	spyOn(API, "getChatModelConfigs").mockImplementation(async () => {
		return state.modelConfigs;
	});
	spyOn(API, "getChatModels").mockImplementation(async () => {
		return state.modelCatalog;
	});

	spyOn(API, "createChatProviderConfig").mockImplementation(async (req) => {
		const created = createProviderConfig({
			id: `provider-${Date.now()}`,
			provider: req.provider,
			display_name: req.display_name ?? "",
			has_api_key: (req.api_key ?? "").trim().length > 0,
			base_url: req.base_url ?? "",
			source: "database",
		});
		state.providerConfigs = [
			...state.providerConfigs.filter((p) => p.provider !== req.provider),
			created,
		];
		return created;
	});

	spyOn(API, "updateChatProviderConfig").mockImplementation(
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

	spyOn(API, "createChatModelConfig").mockImplementation(async (req) => {
		const created = createModelConfig({
			id: `model-${state.modelConfigs.length + 1}`,
			provider: req.provider,
			model: req.model,
			display_name: req.display_name || req.model,
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
		});
		state.modelConfigs = [...state.modelConfigs, created];
		return created;
	});

	spyOn(API, "deleteChatModelConfig").mockImplementation(
		async (modelConfigId) => {
			state.modelConfigs = state.modelConfigs.filter(
				(m) => m.id !== modelConfigId,
			);
		},
	);

	// Unused but mock to avoid errors.
	spyOn(API, "deleteChatProviderConfig").mockResolvedValue(undefined);
	spyOn(API, "updateChatModelConfig").mockResolvedValue(
		createModelConfig({
			id: "stub",
			provider: "stub",
			model: "stub",
		}),
	);
};

// ── Meta ───────────────────────────────────────────────────────

const meta: Meta<typeof ChatModelAdminPanel> = {
	title: "pages/AgentsPage/ChatModelAdminPanel",
	component: ChatModelAdminPanel,
};

export default meta;
type Story = StoryObj<typeof ChatModelAdminPanel>;

// ── Providers section stories ──────────────────────────────────

export const ProviderAccordionCards: Story = {
	args: { section: "providers" as ChatModelAdminSection },
	beforeEach: () => {
		setupChatSpies({
			providerConfigs: [
				createProviderConfig({
					id: nilProviderConfigID,
					provider: "openrouter",
					display_name: "OpenRouter",
					source: "supported",
					enabled: false,
				}),
			],
			modelConfigs: [],
			modelCatalog: { providers: [] },
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await expect(await body.findByText("OpenRouter")).toBeInTheDocument();
		// OpenAI should not be rendered.
		expect(body.queryByText("OpenAI")).not.toBeInTheDocument();

		await userEvent.click(body.getByRole("button", { name: /OpenRouter/i }));
		await expect(body.getByLabelText("Base URL")).toBeInTheDocument();
	},
};

export const EnvPresetProviders: Story = {
	args: { section: "providers" as ChatModelAdminSection },
	beforeEach: () => {
		setupChatSpies({
			providerConfigs: [
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
			modelConfigs: [],
			modelCatalog: { providers: [] },
		});
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
		expect(body.queryByLabelText(/API key/i)).not.toBeInTheDocument();
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
		await userEvent.click(body.getByRole("button", { name: /Anthropic/i }));
		await expect(
			await body.findByText(
				"This provider key is configured from deployment environment settings and cannot be edited in this UI.",
			),
		).toBeVisible();
	},
};

export const CreateAndUpdateProvider: Story = {
	args: { section: "providers" as ChatModelAdminSection },
	beforeEach: () => {
		setupChatSpies({
			providerConfigs: [
				createProviderConfig({
					id: nilProviderConfigID,
					provider: "openai",
					display_name: "OpenAI",
					source: "supported",
					enabled: false,
					has_api_key: false,
				}),
			],
			modelConfigs: [],
			modelCatalog: {
				providers: [
					{
						provider: "openai",
						available: false,
						unavailable_reason: "missing_api_key",
						models: [],
					},
				],
			},
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Navigate to the OpenAI detail view.
		await userEvent.click(await body.findByRole("button", { name: /OpenAI/i }));

		// Fill in form to create a provider config.
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

		// The create spy should have been called.
		await waitFor(() => {
			expect(API.createChatProviderConfig).toHaveBeenCalledTimes(1);
		});
		expect(API.createChatProviderConfig).toHaveBeenCalledWith(
			expect.objectContaining({
				provider: "openai",
				api_key: "sk-provider-key",
				base_url: "https://proxy.example.com/v1",
			}),
		);

		// After creation, queries refetch and the component re-keys
		// because providerConfig now exists. Navigate back to the list
		// and re-enter the detail view to interact with the updated form.
		await waitFor(() => {
			expect(
				body.getByRole("button", { name: "Save changes" }),
			).toBeInTheDocument();
		});

		// The form was re-rendered with the new providerConfig.
		// Focus the API key field, type a new key, update the base URL,
		// and save.
		const apiKeyInput = body.getByLabelText(/API key/i);
		await userEvent.clear(apiKeyInput);
		await userEvent.type(apiKeyInput, "sk-updated-provider-key");
		const baseURLInput = body.getByLabelText("Base URL");
		await userEvent.clear(baseURLInput);
		await userEvent.type(baseURLInput, "https://internal-proxy.example.com/v2");
		await userEvent.click(body.getByRole("button", { name: "Save changes" }));

		await waitFor(() => {
			expect(API.updateChatProviderConfig).toHaveBeenCalledTimes(1);
		});
		expect(API.updateChatProviderConfig).toHaveBeenCalledWith(
			expect.any(String),
			expect.objectContaining({
				api_key: "sk-updated-provider-key",
				base_url: "https://internal-proxy.example.com/v2",
			}),
		);
	},
};

// ── Models section stories ─────────────────────────────────────

/**
 * Helper to open the "Add model" dropdown and select a provider.
 * The "Add model" button is a DropdownMenuTrigger. Clicking it opens
 * a dropdown of addable providers. We then select the given provider.
 */
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

export const NoModelConfigByDefault: Story = {
	args: { section: "models" as ChatModelAdminSection },
	beforeEach: () => {
		setupChatSpies({
			providerConfigs: [
				createProviderConfig({
					id: "provider-openai",
					provider: "openai",
					display_name: "OpenAI",
					source: "database",
					has_api_key: true,
				}),
			],
			modelConfigs: [],
			modelCatalog: { providers: [] },
		});
	},
	play: async ({ canvasElement }) => {
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
			expect(API.createChatModelConfig).toHaveBeenCalledTimes(1);
		});
		expect(API.createChatModelConfig).toHaveBeenCalledWith(
			expect.objectContaining({
				provider: "openai",
				model: "gpt-5-pro",
			}),
		);
		// The request should not include a model_config key.
		const callArgs = (
			API.createChatModelConfig as unknown as ReturnType<typeof spyOn>
		).mock.calls[0][0] as Record<string, unknown>;
		expect(callArgs).not.toHaveProperty("model_config");
	},
};

export const SubmitModelConfigExplicitly: Story = {
	args: { section: "models" as ChatModelAdminSection },
	beforeEach: () => {
		setupChatSpies({
			providerConfigs: [
				createProviderConfig({
					id: "provider-openai",
					provider: "openai",
					display_name: "OpenAI",
					source: "database",
					has_api_key: true,
				}),
			],
			modelConfigs: [],
			modelCatalog: { providers: [] },
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Open "Add model" dropdown and select the OpenAI provider.
		await openAddModelForm(body, "OpenAI");

		await userEvent.type(
			body.getByLabelText(/Model Identifier/i),
			"gpt-5-pro-custom",
		);
		await userEvent.type(body.getByLabelText(/Context limit/i), "200000");
		// Max output tokens and provider options are under "Advanced".
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
			expect(API.createChatModelConfig).toHaveBeenCalledTimes(1);
		});
		expect(API.createChatModelConfig).toHaveBeenCalledWith(
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

// ── Per-provider model form stories ────────────────────────────
// Each story opens the "Add model" form for a specific provider
// so you can visually verify the schema-driven fields render.

const providerFormSetup = (provider: string, displayName: string) => ({
	args: { section: "models" as ChatModelAdminSection },
	beforeEach: () => {
		setupChatSpies({
			providerConfigs: [
				createProviderConfig({
					id: `provider-${provider}`,
					provider,
					display_name: displayName,
					source: "database",
					has_api_key: true,
				}),
			],
			modelConfigs: [],
			modelCatalog: { providers: [] },
		});
	},
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
		// Bedrock aliases to Anthropic fields.
		await expect(
			await body.findByLabelText(/Send Reasoning/i),
		).toBeInTheDocument();
		await expect(await body.findByLabelText(/Effort/i)).toBeInTheDocument();
	},
};

export const ValidatesModelConfigFields: Story = {
	args: { section: "models" as ChatModelAdminSection },
	beforeEach: () => {
		setupChatSpies({
			providerConfigs: [
				createProviderConfig({
					id: "provider-openai",
					provider: "openai",
					display_name: "OpenAI",
					source: "database",
					has_api_key: true,
				}),
			],
			modelConfigs: [],
			modelCatalog: { providers: [] },
		});
	},
	play: async ({ canvasElement }) => {
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
		// No API call should have been made.
		expect(API.createChatModelConfig).not.toHaveBeenCalled();
	},
};
