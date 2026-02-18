import type {
	ChatModelConfig,
	ChatModelsResponse,
	ChatProviderConfig,
} from "api/api";
import { HttpResponse, http } from "msw";
import { QueryClientProvider } from "react-query";
import {
	createTestQueryClient,
	renderComponent,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ChatModelAdminPanel } from "./ChatModelAdminPanel";

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
	overrides: Partial<ChatProviderConfig> & Pick<ChatProviderConfig, "id" | "provider">,
): ChatProviderConfig => {
	return {
		id: overrides.id,
		provider: overrides.provider,
		display_name: overrides.display_name ?? "",
		enabled: overrides.enabled ?? true,
		has_api_key: overrides.has_api_key ?? false,
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
			const createdProvider = createProviderConfig({
				id: `provider-${Date.now()}`,
				provider,
				display_name: displayName,
				has_api_key: apiKey.length > 0,
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
				has_api_key:
					typeof body.api_key === "string"
						? body.api_key.trim().length > 0
						: currentProvider.has_api_key,
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

const renderPanel = () => {
	const queryClient = createTestQueryClient();
	return renderComponent(
		<QueryClientProvider client={queryClient}>
			<ChatModelAdminPanel />
		</QueryClientProvider>,
	);
};

describe(ChatModelAdminPanel.name, () => {
	it("renders provider options from backend provider data", async () => {
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

		renderPanel();

		expect(await screen.findByText("OpenRouter")).toBeInTheDocument();
		expect(screen.queryByText("OpenAI")).not.toBeInTheDocument();
	});

	it("shows env presets distinctly and allows BYO provider config", async () => {
		installChatHandlers({
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
			modelCatalog: {
				providers: [],
			},
		});

		renderPanel();

		expect(await screen.findByText("Environment preset detected.")).toBeVisible();
		expect(screen.getByText("Anthropic")).toBeInTheDocument();
		expect(
			screen.getByText("Create a managed provider config before adding models."),
		).toBeVisible();
		const createProviderButton = screen.getByRole("button", {
			name: "Create provider config",
		});
		expect(createProviderButton).toBeDisabled();

		await userEvent.type(screen.getByLabelText("API key"), "sk-override");
		expect(createProviderButton).toBeEnabled();
	});

	it("creates provider config, updates models, and sends update provider requests", async () => {
		const state: ChatAdminState = {
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
		};
		const log: RequestLog = {
			createProviderBodies: [],
			updateProviderBodies: [],
			createModelBodies: [],
		};
		installChatHandlers(state, log);

		renderPanel();

		await userEvent.type(
			await screen.findByLabelText("API key"),
			"sk-provider-key",
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
		});

		await waitFor(() => {
			expect(
				screen.getByRole("button", { name: "Save provider changes" }),
			).toBeInTheDocument();
		});

		const displayNameInput = screen.getByPlaceholderText(
			"Friendly provider label",
		);
		await userEvent.clear(displayNameInput);
		await userEvent.type(displayNameInput, "Primary OpenAI");
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
		});

		await userEvent.type(screen.getByLabelText("Model ID"), "gpt-5-mini");
		await userEvent.click(screen.getByRole("button", { name: "Add model" }));

		await waitFor(() => {
			expect(log.createModelBodies).toHaveLength(1);
		});
		expect(log.createModelBodies[0]).toMatchObject({
			provider: "openai",
			model: "gpt-5-mini",
		});
		expect(await screen.findAllByText("gpt-5-mini")).not.toHaveLength(0);

		await userEvent.click(screen.getByRole("button", { name: "Remove model" }));
		await waitFor(() => {
			expect(screen.queryAllByText("gpt-5-mini")).toHaveLength(0);
		});
	});
});
