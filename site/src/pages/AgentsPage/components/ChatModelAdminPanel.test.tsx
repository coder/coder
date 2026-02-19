import type {
	ChatModelConfig,
	ChatModelsResponse,
	ChatProviderConfig,
} from "api/api";
import { HttpResponse, http } from "msw";
import { QueryClientProvider } from "react-query";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
	createTestQueryClient,
	renderComponent,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import {
	ChatModelAdminPanel,
	type ChatModelAdminSection,
} from "./ChatModelAdminPanel";

const now = "2026-02-18T12:00:00.000Z";
const nilProviderConfigID = "00000000-0000-0000-0000-000000000000";

type ChatAdminState = {
	providerConfigs: ChatProviderConfig[];
	modelConfigs: ChatModelConfig[];
	modelCatalog: ChatModelsResponse;
};

type RequestLog = {
	createProviderBodies: Array<Record<string, unknown>>;
	updateProviderBodies: Array<{
		providerConfigId: string;
		body: Record<string, unknown>;
	}>;
	createModelBodies: Array<Record<string, unknown>>;
};

const createProviderConfig = (
	overrides: Partial<ChatProviderConfig> &
		Pick<ChatProviderConfig, "id" | "provider">,
): ChatProviderConfig => {
	const hasAPIKey = Boolean(overrides.api_key_set ?? overrides.has_api_key);
	return {
		id: overrides.id,
		provider: overrides.provider,
		display_name: overrides.display_name ?? "",
		enabled: overrides.enabled ?? true,
		api_key_set: hasAPIKey,
		has_api_key: hasAPIKey,
		base_url: overrides.base_url ?? "",
		source: overrides.source ?? "database",
		created_at: overrides.created_at ?? now,
		updated_at: overrides.updated_at ?? now,
	};
};

const createModelConfig = (
	overrides: Partial<ChatModelConfig> &
		Pick<ChatModelConfig, "id" | "provider" | "model">,
): ChatModelConfig => {
	return {
		id: overrides.id,
		provider: overrides.provider,
		model: overrides.model,
		display_name: overrides.display_name ?? overrides.model,
		enabled: overrides.enabled ?? true,
		created_at: overrides.created_at ?? now,
		updated_at: overrides.updated_at ?? now,
	};
};

const installChatHandlers = (state: ChatAdminState, log?: RequestLog) => {
	server.use(
		http.get("/api/v2/chats/providers", () => {
			return HttpResponse.json(state.providerConfigs);
		}),
		http.post("/api/v2/chats/providers", async ({ request }) => {
			const body = (await request.json()) as Record<string, unknown>;
			log?.createProviderBodies.push(body);

			const provider = `${body.provider ?? ""}`.trim();
			const apiKey = `${body.api_key ?? ""}`.trim();
			const displayName = `${body.display_name ?? ""}`.trim();
			const baseURL =
				typeof body.base_url === "string" ? body.base_url.trim() : "";
			const createdProvider = createProviderConfig({
				id: `provider-${Date.now()}`,
				provider,
				display_name: displayName,
				api_key_set: apiKey.length > 0,
				base_url: baseURL,
				source: "database",
			});

			state.providerConfigs = [
				...state.providerConfigs.filter(
					(providerConfig) => providerConfig.provider !== provider,
				),
				createdProvider,
			];

			return HttpResponse.json(createdProvider, { status: 201 });
		}),
		http.patch("/api/v2/chats/providers/:providerConfigId", async ({
			params,
			request,
		}) => {
			const providerConfigId = `${params.providerConfigId ?? ""}`;
			const body = (await request.json()) as Record<string, unknown>;
			log?.updateProviderBodies.push({ providerConfigId, body });

			const providerIndex = state.providerConfigs.findIndex(
				(providerConfig) => providerConfig.id === providerConfigId,
			);
			if (providerIndex < 0) {
				return HttpResponse.json(
					{ message: "Provider config not found." },
					{ status: 404 },
				);
			}

			const currentProvider = state.providerConfigs[providerIndex];
			const updatedProvider: ChatProviderConfig = {
				...currentProvider,
				display_name:
					typeof body.display_name === "string"
						? body.display_name
						: currentProvider.display_name,
				api_key_set:
					typeof body.api_key === "string"
						? body.api_key.trim().length > 0
						: currentProvider.api_key_set,
				has_api_key:
					typeof body.api_key === "string"
						? body.api_key.trim().length > 0
						: currentProvider.has_api_key,
				base_url:
					typeof body.base_url === "string"
						? body.base_url
						: currentProvider.base_url,
				updated_at: now,
			};
			state.providerConfigs = state.providerConfigs.map((providerConfig, index) =>
				index === providerIndex ? updatedProvider : providerConfig,
			);

			return HttpResponse.json(updatedProvider);
		}),
		http.get("/api/v2/chats/model-configs", () => {
			return HttpResponse.json(state.modelConfigs);
		}),
		http.post("/api/v2/chats/model-configs", async ({ request }) => {
			const body = (await request.json()) as Record<string, unknown>;
			log?.createModelBodies.push(body);

			const provider = `${body.provider ?? ""}`.trim();
			const model = `${body.model ?? ""}`.trim();
			const displayName = `${body.display_name ?? ""}`.trim();
			const createdModel = createModelConfig({
				id: `model-${state.modelConfigs.length + 1}`,
				provider,
				model,
				display_name: displayName || model,
			});
			state.modelConfigs = [...state.modelConfigs, createdModel];

			return HttpResponse.json(createdModel, { status: 201 });
		}),
		http.delete("/api/v2/chats/model-configs/:modelConfigId", ({ params }) => {
			const modelConfigId = `${params.modelConfigId ?? ""}`;
			state.modelConfigs = state.modelConfigs.filter(
				(modelConfig) => modelConfig.id !== modelConfigId,
			);
			return new HttpResponse(null, { status: 204 });
		}),
		http.get("/api/v2/chats/models", () => {
			return HttpResponse.json(state.modelCatalog);
		}),
	);
};

const renderPanel = (section: ChatModelAdminSection = "providers") => {
	const queryClient = createTestQueryClient();
	return renderComponent(
		<QueryClientProvider client={queryClient}>
			<ChatModelAdminPanel section={section} />
		</QueryClientProvider>,
	);
};

describe(ChatModelAdminPanel.name, () => {
	it("renders provider accordion cards from backend provider data", async () => {
		installChatHandlers({
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
			modelCatalog: {
				providers: [],
			},
		});

		renderPanel("providers");

		expect(await screen.findByText("OpenRouter")).toBeInTheDocument();
		expect(screen.queryByText("OpenAI")).not.toBeInTheDocument();
		expect(screen.getByLabelText("Base URL (optional)")).toBeInTheDocument();
	});

	it("shows env presets distinctly and blocks API key editing", async () => {
		installChatHandlers({
			providerConfigs: [
				createProviderConfig({
					id: nilProviderConfigID,
					provider: "openai",
					display_name: "OpenAI",
					api_key_set: true,
					source: "env_preset",
					enabled: true,
				}),
				createProviderConfig({
					id: nilProviderConfigID,
					provider: "anthropic",
					display_name: "Anthropic",
					api_key_set: true,
					source: "env_preset",
					enabled: true,
				}),
			],
			modelConfigs: [],
			modelCatalog: {
				providers: [],
			},
		});

		renderPanel("providers");

		expect(
			await screen.findByText("API key managed by environment variable."),
		).toBeVisible();
		expect(screen.getByText("Anthropic")).toBeInTheDocument();
		expect(
			screen.getByText(
				"This provider API key is managed by an environment variable.",
			),
		).toBeVisible();
		expect(screen.queryByLabelText("API key")).not.toBeInTheDocument();
		expect(
			screen.queryByRole("button", { name: "Create provider config" }),
		).not.toBeInTheDocument();
	});

	it("creates and updates provider configs with base URL values", async () => {
		const state: ChatAdminState = {
			providerConfigs: [
				createProviderConfig({
					id: nilProviderConfigID,
					provider: "openai",
					display_name: "OpenAI",
					source: "supported",
					enabled: false,
					api_key_set: false,
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
		};
		const log: RequestLog = {
			createProviderBodies: [],
			updateProviderBodies: [],
			createModelBodies: [],
		};
		installChatHandlers(state, log);

		renderPanel("providers");

		await userEvent.type(await screen.findByLabelText("API key"), "sk-provider-key");
		await userEvent.type(
			screen.getByLabelText("Base URL (optional)"),
			"https://proxy.example.com/v1",
		);
		await userEvent.click(
			screen.getByRole("button", { name: "Create provider config" }),
		);

		await waitFor(() => {
			expect(log.createProviderBodies).toHaveLength(1);
		});
		expect(log.createProviderBodies[0]).toMatchObject({
			provider: "openai",
			api_key: "sk-provider-key",
			base_url: "https://proxy.example.com/v1",
		});

		await waitFor(() => {
			expect(
				screen.getByRole("button", { name: "Save provider changes" }),
			).toBeInTheDocument();
		});

		const displayNameInput = screen.getByPlaceholderText("Friendly provider label");
		await userEvent.clear(displayNameInput);
		await userEvent.type(displayNameInput, "Primary OpenAI");
		const baseURLInput = screen.getByLabelText("Base URL (optional)");
		await userEvent.clear(baseURLInput);
		await userEvent.type(baseURLInput, "https://internal-proxy.example.com/v2");
		await userEvent.type(
			screen.getByLabelText("API key (optional)"),
			"sk-updated-provider-key",
		);
		await userEvent.click(
			screen.getByRole("button", { name: "Save provider changes" }),
		);

		await waitFor(() => {
			expect(log.updateProviderBodies).toHaveLength(1);
		});
		expect(log.updateProviderBodies[0].body).toMatchObject({
			display_name: "Primary OpenAI",
			api_key: "sk-updated-provider-key",
			base_url: "https://internal-proxy.example.com/v2",
		});
	});

	it("models section lists all configured models and supports provider-scoped creation", async () => {
		const state: ChatAdminState = {
			providerConfigs: [
				createProviderConfig({
					id: "provider-openai",
					provider: "openai",
					display_name: "OpenAI",
					source: "database",
					api_key_set: true,
				}),
				createProviderConfig({
					id: nilProviderConfigID,
					provider: "anthropic",
					display_name: "Anthropic",
					source: "supported",
					api_key_set: false,
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
			modelCatalog: {
				providers: [],
			},
		};
		const log: RequestLog = {
			createProviderBodies: [],
			updateProviderBodies: [],
			createModelBodies: [],
		};
		installChatHandlers(state, log);

		renderPanel("models");

		expect(await screen.findByLabelText("Provider for new model")).toBeInTheDocument();
		await waitFor(() => {
			expect(
				screen.getByRole("combobox", { name: "Provider for new model" }),
			).not.toBeDisabled();
		});
		expect(await screen.findAllByText("gpt-5-mini")).not.toHaveLength(0);
		expect(await screen.findAllByText("claude-sonnet-4-5")).not.toHaveLength(0);

		await userEvent.type(screen.getByLabelText("Model ID"), "gpt-5-nano");
		await userEvent.click(screen.getByRole("button", { name: "Add model" }));

		await waitFor(() => {
			expect(log.createModelBodies).toHaveLength(1);
		});
		expect(log.createModelBodies[0]).toMatchObject({
			provider: "openai",
			model: "gpt-5-nano",
		});
		expect(await screen.findAllByText("gpt-5-nano")).not.toHaveLength(0);

		const nanoRowLabel = (await screen.findAllByText("gpt-5-nano"))[0];
		const nanoRow = nanoRowLabel.closest(
			"div.flex.items-start.justify-between",
		);
		expect(nanoRow).not.toBeNull();
		await userEvent.click(
			within(nanoRow as HTMLElement).getByRole("button", {
				name: "Remove model",
			}),
		);
		await waitFor(() => {
			expect(screen.queryAllByText("gpt-5-nano")).toHaveLength(0);
		});

		await userEvent.click(
			screen.getByRole("combobox", { name: "Provider for new model" }),
		);
		await userEvent.click(screen.getByRole("option", { name: "Anthropic" }));
		expect(
			await screen.findByText(
				"Create a managed provider config before adding models.",
			),
		).toBeVisible();
	});
});
