import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import type { ChatModel, UserChatProviderConfig } from "#/api/typesGenerated";
import { MockChatModel } from "#/testHelpers/chatModels";
import { AgentSettingsAPIKeysPageView } from "./AgentSettingsAPIKeysPageView";

const createProvider = (
	overrides: Partial<UserChatProviderConfig> &
		Pick<UserChatProviderConfig, "provider_id" | "provider">,
): UserChatProviderConfig => ({
	provider_id: overrides.provider_id,
	provider: overrides.provider,
	display_name: overrides.display_name ?? overrides.provider,
	icon: overrides.icon ?? "",
	enabled: overrides.enabled ?? true,
	has_user_api_key: overrides.has_user_api_key ?? false,
	has_central_api_key_fallback: overrides.has_central_api_key_fallback ?? false,
	byok_enabled: overrides.byok_enabled ?? true,
});

const createModel = (
	overrides: Partial<ChatModel> &
		Pick<ChatModel, "id" | "ai_provider_id" | "model">,
): ChatModel => ({
	...MockChatModel,
	created_at: "2026-03-01T00:00:00.000Z",
	updated_at: "2026-03-01T00:00:00.000Z",
	...overrides,
});

const baseProvider = createProvider({
	provider_id: "prov-1",
	provider: "openai",
	display_name: "OpenAI",
});

const baseModel = createModel({
	id: "model-1",
	ai_provider_id: "prov-1",
	display_name: "GPT-4o",
	model: "gpt-4o",
});

const baseModels = [baseModel];

const meta = {
	title: "pages/AgentsPage/AgentSettingsAPIKeysPageView",
	component: AgentSettingsAPIKeysPageView,
	args: {
		error: undefined,
		isLoading: false,
		providers: [baseProvider],
		models: baseModels,
		isModelsLoading: false,
		areModelsUnavailable: false,
	},
} satisfies Meta<typeof AgentSettingsAPIKeysPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsAPIKeysPageView>;

export const Default: Story = {};

export const WithSavedKey: Story = {
	args: {
		providers: [
			createProvider({
				provider_id: "prov-1",
				provider: "openai",
				display_name: "OpenAI",
				has_user_api_key: true,
				has_central_api_key_fallback: true,
			}),
		],
		models: baseModels,
	},
};

export const UserKeysDisabled: Story = {
	args: {
		providers: [
			createProvider({
				provider_id: "prov-1",
				provider: "openai",
				display_name: "OpenAI",
				byok_enabled: false,
				has_central_api_key_fallback: true,
			}),
		],
	},
};

export const WithFallback: Story = {
	args: {
		providers: [
			createProvider({
				provider_id: "prov-1",
				provider: "anthropic",
				display_name: "Anthropic",
				has_central_api_key_fallback: true,
			}),
		],
		models: [
			createModel({
				id: "model-1",
				ai_provider_id: "prov-1",
				display_name: "Claude Sonnet 4",
				model: "claude-sonnet-4-20250514",
			}),
		],
	},
};

export const MultipleProviders: Story = {
	args: {
		providers: [
			createProvider({
				provider_id: "prov-openai",
				provider: "openai",
				display_name: "OpenAI",
				has_user_api_key: true,
				has_central_api_key_fallback: true,
			}),
			createProvider({
				provider_id: "prov-anthropic",
				provider: "anthropic",
				display_name: "Anthropic",
				has_central_api_key_fallback: true,
			}),
			createProvider({
				provider_id: "prov-google",
				provider: "google",
				display_name: "Google",
			}),
		],
		models: [
			createModel({
				id: "model-openai-1",
				ai_provider_id: "prov-openai",
				display_name: "GPT-4o",
				model: "gpt-4o",
			}),
			createModel({
				id: "model-anthropic-1",
				ai_provider_id: "prov-anthropic",
				display_name: "Claude Sonnet 4",
				model: "claude-sonnet-4-20250514",
			}),
			createModel({
				id: "model-anthropic-2",
				ai_provider_id: "prov-anthropic",
				display_name: "Claude Opus 4",
				model: "claude-opus-4-20250514",
			}),
		],
	},
};

export const Empty: Story = {
	args: {
		providers: [],
		models: [],
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
		providers: [],
		models: [],
	},
};

export const ModelsUnavailable: Story = {
	args: {
		areModelsUnavailable: true,
		models: [],
	},
};

export const ModelsLoading: Story = {
	args: {
		isModelsLoading: true,
		models: [],
	},
};

export const SomeModelsUnavailable: Story = {
	args: {
		areModelsUnavailable: true,
	},
};

export const SavingSingleProvider: Story = {
	args: {
		providers: [
			baseProvider,
			createProvider({
				provider_id: "prov-2",
				provider: "anthropic",
				display_name: "Anthropic",
			}),
		],
		models: [
			...baseModels,
			createModel({
				id: "model-2",
				ai_provider_id: "prov-2",
				display_name: "Claude Sonnet 4",
				model: "claude-sonnet-4-20250514",
			}),
		],
	},
	play: async ({ canvasElement }) => {
		spyOn(API.experimental, "upsertUserAIProviderKey").mockImplementation(
			() => new Promise(() => {}),
		);
		const canvas = within(canvasElement);
		const panel = within(
			await canvas.findByRole("article", { name: "OpenAI" }),
		);
		await userEvent.type(panel.getByLabelText("API Key"), "sk-test-key");
		await userEvent.click(panel.getByRole("button", { name: "Save" }));
	},
};

export const RemovesProviderKey: Story = {
	args: {
		providers: [
			createProvider({
				provider_id: "prov-1",
				provider: "openai",
				display_name: "OpenAI",
				has_user_api_key: true,
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Remove" }),
		);
	},
};

export const ClearsMaskedApiKeyOnFocus: Story = {
	args: {
		providers: [
			createProvider({
				provider_id: "prov-1",
				provider: "openai",
				display_name: "OpenAI",
				has_user_api_key: true,
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(await canvas.findByLabelText("API Key"));
	},
};

export const ShowsProviderStatuses: Story = {
	args: {
		providers: [
			createProvider({
				provider_id: "prov-openai",
				provider: "openai",
				display_name: "OpenAI",
				has_user_api_key: true,
				has_central_api_key_fallback: false,
			}),
			createProvider({
				provider_id: "prov-anthropic",
				provider: "anthropic",
				display_name: "Anthropic",
				has_user_api_key: false,
				has_central_api_key_fallback: true,
			}),
			createProvider({
				provider_id: "prov-google",
				provider: "google",
				display_name: "Google",
				has_user_api_key: false,
				has_central_api_key_fallback: false,
			}),
		],
		models: [
			createModel({
				id: "model-openai-1",
				ai_provider_id: "prov-openai",
				display_name: "GPT-4o",
				model: "gpt-4o",
			}),
			createModel({
				id: "model-anthropic-1",
				ai_provider_id: "prov-anthropic",
				display_name: "Claude Sonnet 4",
				model: "claude-sonnet-4-20250514",
			}),
			createModel({
				id: "model-google-1",
				ai_provider_id: "prov-google",
				display_name: "Gemini 2.5 Pro",
				model: "gemini-2.5-pro",
			}),
		],
	},
};

export const WithError: Story = {
	args: {
		error: new Error("Failed to load provider configurations"),
	},
};
