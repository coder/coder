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
			...state.providerConfigs.filter(
				(p) => p.provider !== req.provider,
			),
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
					typeof req.base_url === "string"
						? req.base_url
						: current.base_url,
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
		await expect(
			await body.findByText("OpenRouter"),
		).toBeInTheDocument();
		// OpenAI should not be rendered.
		expect(body.queryByText("OpenAI")).not.toBeInTheDocument();

		await userEvent.click(
			body.getByRole("button", { name: /OpenRouter/i }),
		);
		await expect(
			body.getByLabelText("Base URL (optional)"),
		).toBeInTheDocument();
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
		await userEvent.click(
			await body.findByRole("button", { name: /OpenAI/i }),
		);
		await expect(
			await body.findByText(
				"API key managed by environment variable.",
			),
		).toBeVisible();
		expect(body.getByText("Anthropic")).toBeInTheDocument();
		expect(
			body.getByText(
				"This provider API key is managed by an environment variable.",
			),
		).toBeVisible();
		expect(
			body.getByText(
				"This provider key is configured from deployment environment settings and cannot be edited in this UI.",
			),
		).toBeVisible();
		expect(body.queryByLabelText("API key")).not.toBeInTheDocument();
		expect(
			body.queryByRole("button", {
				name: "Create provider config",
			}),
		).not.toBeInTheDocument();
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

		// Expand the accordion.
		await userEvent.click(
			await body.findByRole("button", { name: /OpenAI/i }),
		);

		// Fill in form to create a provider config.
		await userEvent.type(
			await body.findByLabelText("API key"),
			"sk-provider-key",
		);
		await userEvent.type(
			body.getByLabelText("Base URL (optional)"),
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

		// After creation the form should switch to "Save changes".
		await waitFor(() => {
			expect(
				body.getByRole("button", { name: "Save changes" }),
			).toBeInTheDocument();
		});

		// Update the display name and base URL.
		const displayNameInput = body.getByPlaceholderText(
			"Friendly provider label",
		);
		await userEvent.clear(displayNameInput);
		await userEvent.type(displayNameInput, "Primary OpenAI");
		const baseURLInput = body.getByLabelText("Base URL (optional)");
		await userEvent.clear(baseURLInput);
		await userEvent.type(
			baseURLInput,
			"https://internal-proxy.example.com/v2",
		);
		await userEvent.type(
			body.getByLabelText("API key (optional)"),
			"sk-updated-provider-key",
		);
		await userEvent.click(
			body.getByRole("button", { name: "Save changes" }),
		);

		await waitFor(() => {
			expect(API.updateChatProviderConfig).toHaveBeenCalledTimes(1);
		});
		expect(API.updateChatProviderConfig).toHaveBeenCalledWith(
			expect.any(String),
			expect.objectContaining({
				display_name: "Primary OpenAI",
				api_key: "sk-updated-provider-key",
				base_url: "https://internal-proxy.example.com/v2",
			}),
		);
	},
};

// ── Models section stories ─────────────────────────────────────

export const ModelsListAndCreate: Story = {
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
				createProviderConfig({
					id: nilProviderConfigID,
					provider: "anthropic",
					display_name: "Anthropic",
					source: "supported",
					has_api_key: false,
				}),
			],
			modelConfigs: [
				createModelConfig({
					id: "model-initial",
					provider: "openai",
					model: "gpt-5-mini",
				}),
				createModelConfig({
					id: "model-anthropic",
					provider: "anthropic",
					model: "claude-sonnet-4-5",
				}),
			],
			modelCatalog: { providers: [] },
		});
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Open Add model dialog.
		await userEvent.click(
			await body.findByRole("button", { name: "Add model" }),
		);
		await expect(
			await body.findByLabelText("Provider"),
		).toBeInTheDocument();
		await waitFor(() => {
			expect(
				body.getByRole("combobox", { name: "Provider" }),
			).not.toBeDisabled();
		});
		expect(
			(await body.findAllByText("gpt-5-mini")).length,
		).toBeGreaterThan(0);
		expect(
			(await body.findAllByText("claude-sonnet-4-5")).length,
		).toBeGreaterThan(0);

		// Fill in form and submit.
		await userEvent.type(
			body.getByLabelText("Model ID"),
			"gpt-5-nano",
		);
		await userEvent.type(
			body.getByLabelText("Context limit"),
			"200000",
		);
		await userEvent.click(
			body.getByRole("button", { name: "Add model" }),
		);

		await waitFor(() => {
			expect(API.createChatModelConfig).toHaveBeenCalledTimes(1);
		});
		expect(API.createChatModelConfig).toHaveBeenCalledWith(
			expect.objectContaining({
				provider: "openai",
				model: "gpt-5-nano",
				context_limit: 200000,
				compression_threshold: 70,
			}),
		);
		expect(
			(await body.findAllByText("gpt-5-nano")).length,
		).toBeGreaterThan(0);

		// Delete the newly-added model row.
		const nanoRowLabel = (await body.findAllByText("gpt-5-nano"))[0];
		const nanoRow = nanoRowLabel.closest("div.group");
		expect(nanoRow).not.toBeNull();
		await userEvent.click(
			within(nanoRow as HTMLElement).getByRole("button", {
				name: "Remove model",
			}),
		);
		await waitFor(() => {
			expect(body.queryAllByText("gpt-5-nano")).toHaveLength(0);
		});

		// Verify that unconfigured provider is disabled in selector.
		await userEvent.click(
			body.getByRole("button", { name: "Add model" }),
		);
		await userEvent.click(
			body.getByRole("combobox", { name: "Provider" }),
		);
		const anthropicOption = await body.findByRole("option", {
			name: /Anthropic/i,
		});
		expect(anthropicOption).toHaveTextContent("(not configured)");
		expect(anthropicOption).toHaveAttribute("aria-disabled", "true");
	},
};

export const ProviderSpecificModelConfigSchema: Story = {
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
				createProviderConfig({
					id: "provider-anthropic",
					provider: "anthropic",
					display_name: "Anthropic",
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

		await userEvent.click(
			await body.findByRole("button", { name: "Add model" }),
		);

		const schemaBlock = await body.findByTestId(
			"chat-model-config-schema",
		);
		expect(schemaBlock).toHaveTextContent('"provider": "openai"');
		expect(schemaBlock).toHaveTextContent('"openai": {');
		expect(schemaBlock).toHaveTextContent('"reasoning_effort": "high"');

		// Switch provider to Anthropic.
		await userEvent.click(
			body.getByRole("combobox", { name: "Provider" }),
		);
		await userEvent.click(
			await body.findByRole("option", { name: /Anthropic/i }),
		);

		await waitFor(() => {
			expect(
				body.getByTestId("chat-model-config-schema"),
			).toHaveTextContent('"provider": "anthropic"');
		});
		expect(
			body.getByTestId("chat-model-config-schema"),
		).toHaveTextContent('"anthropic": {');
		expect(
			body.getByTestId("chat-model-config-schema"),
		).toHaveTextContent('"thinking": {');
	},
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

		await userEvent.click(
			await body.findByRole("button", { name: "Add model" }),
		);
		await userEvent.type(
			body.getByLabelText("Model ID"),
			"gpt-5-pro",
		);
		await userEvent.type(
			body.getByLabelText("Context limit"),
			"200000",
		);

		await expect(
			await body.findByLabelText("Max output tokens (optional)"),
		).toHaveValue("");

		await userEvent.click(
			body.getByRole("button", { name: "Add model" }),
		);
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
			API.createChatModelConfig as ReturnType<typeof spyOn>
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

		await userEvent.click(
			await body.findByRole("button", { name: "Add model" }),
		);
		await userEvent.type(
			body.getByLabelText("Model ID"),
			"gpt-5-pro-custom",
		);
		await userEvent.type(
			body.getByLabelText("Context limit"),
			"200000",
		);
		await userEvent.type(
			await body.findByLabelText("Max output tokens (optional)"),
			"32000",
		);
		await userEvent.click(
			body.getByRole("combobox", {
				name: "Reasoning effort (optional)",
			}),
		);
		await userEvent.click(
			await body.findByRole("option", { name: "high" }),
		);

		await userEvent.click(
			body.getByRole("button", { name: "Add model" }),
		);
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

		await userEvent.click(
			await body.findByRole("button", { name: "Add model" }),
		);
		await userEvent.type(
			body.getByLabelText("Model ID"),
			"gpt-5-pro",
		);
		await userEvent.type(
			body.getByLabelText("Context limit"),
			"200000",
		);
		await userEvent.type(
			await body.findByLabelText("Max output tokens (optional)"),
			"not-a-number",
		);
		expect(
			body.getByText("Max output tokens must be a valid number."),
		).toBeInTheDocument();
		expect(
			body.getByRole("button", { name: "Add model" }),
		).toBeDisabled();
		// No API call should have been made.
		expect(API.createChatModelConfig).not.toHaveBeenCalled();
	},
};
