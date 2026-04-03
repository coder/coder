import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type {
	ChatModelConfig,
	UserChatProviderConfig,
} from "#/api/typesGenerated";
import {
	AgentSettingsAPIKeysPageView,
	type AgentSettingsAPIKeysPageViewProps,
} from "./AgentSettingsAPIKeysPageView";

const createProvider = (
	overrides: Partial<UserChatProviderConfig> &
		Pick<UserChatProviderConfig, "provider_id" | "provider">,
): UserChatProviderConfig => ({
	provider_id: overrides.provider_id,
	provider: overrides.provider,
	display_name: overrides.display_name ?? overrides.provider,
	has_user_api_key: overrides.has_user_api_key ?? false,
	has_central_api_key_fallback: overrides.has_central_api_key_fallback ?? false,
});

const createModel = (
	overrides: Partial<ChatModelConfig> &
		Pick<ChatModelConfig, "id" | "provider" | "model">,
): ChatModelConfig => ({
	id: overrides.id,
	provider: overrides.provider,
	display_name: overrides.display_name ?? overrides.model,
	model: overrides.model,
	enabled: overrides.enabled ?? true,
	is_default: overrides.is_default ?? false,
	context_limit: overrides.context_limit ?? 200000,
	compression_threshold: overrides.compression_threshold ?? 70,
	model_config: overrides.model_config,
	created_at: overrides.created_at ?? "2026-03-01T00:00:00.000Z",
	updated_at: overrides.updated_at ?? "2026-03-01T00:00:00.000Z",
});

const baseProvider = createProvider({
	provider_id: "prov-1",
	provider: "openai",
	display_name: "OpenAI",
});

const baseModel = createModel({
	id: "model-1",
	provider: "openai",
	display_name: "GPT-4o",
	model: "gpt-4o",
});

const baseModels = [baseModel];

const createProviderItems = (
	providers: readonly UserChatProviderConfig[],
): AgentSettingsAPIKeysPageViewProps["providerItems"] => {
	return providers.map((provider) => ({
		provider,
		renderKey: `${provider.provider_id}-${provider.has_user_api_key}`,
		isSaving: false,
		isRemoving: false,
	}));
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsAPIKeysPageView",
	component: AgentSettingsAPIKeysPageView,
	args: {
		error: undefined,
		isLoading: false,
		providerItems: createProviderItems([baseProvider]),
		models: baseModels,
		isModelsLoading: false,
		areModelsUnavailable: false,
		onSave: fn(),
		onRemove: fn(),
	},
} satisfies Meta<typeof AgentSettingsAPIKeysPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsAPIKeysPageView>;

export const Default: Story = {};

export const WithSavedKey: Story = {
	args: {
		providerItems: createProviderItems([
			createProvider({
				provider_id: "prov-1",
				provider: "openai",
				display_name: "OpenAI",
				has_user_api_key: true,
				has_central_api_key_fallback: true,
			}),
		]),
		models: baseModels,
	},
};

export const MasksApiKeyInput: Story = {
	args: {
		providerItems: createProviderItems([
			createProvider({
				provider_id: "prov-1",
				provider: "openai",
				display_name: "OpenAI",
				has_user_api_key: true,
			}),
		]),
		models: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByLabelText(/API Key/i)).toHaveAttribute(
			"type",
			"password",
		);
	},
};

export const WithFallback: Story = {
	args: {
		providerItems: createProviderItems([
			createProvider({
				provider_id: "prov-1",
				provider: "anthropic",
				display_name: "Anthropic",
				has_central_api_key_fallback: true,
			}),
		]),
		models: [
			createModel({
				id: "model-1",
				provider: "anthropic",
				display_name: "Claude Sonnet 4",
				model: "claude-sonnet-4-20250514",
			}),
		],
	},
};

export const MultipleProviders: Story = {
	args: {
		providerItems: createProviderItems([
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
		]),
		models: [
			createModel({
				id: "model-openai-1",
				provider: "openai",
				display_name: "GPT-4o",
				model: "gpt-4o",
			}),
			createModel({
				id: "model-anthropic-1",
				provider: "anthropic",
				display_name: "Claude Sonnet 4",
				model: "claude-sonnet-4-20250514",
			}),
			createModel({
				id: "model-anthropic-2",
				provider: "anthropic",
				display_name: "Claude Opus 4",
				model: "claude-opus-4-20250514",
			}),
		],
	},
};

export const Empty: Story = {
	args: {
		providerItems: [],
		models: [],
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
		providerItems: [],
		models: [],
	},
};

export const ModelsUnavailable: Story = {
	args: {
		areModelsUnavailable: true,
		models: [],
	},
};

export const SavingSingleProvider: Story = {
	args: {
		providerItems: [
			{
				provider: baseProvider,
				renderKey: `${baseProvider.provider_id}-${baseProvider.has_user_api_key}`,
				isSaving: true,
				isRemoving: false,
			},
			{
				provider: createProvider({
					provider_id: "prov-2",
					provider: "anthropic",
					display_name: "Anthropic",
				}),
				renderKey: "prov-2-false",
				isSaving: false,
				isRemoving: false,
			},
		],
		models: [
			...baseModels,
			createModel({
				id: "model-2",
				provider: "anthropic",
				display_name: "Claude Sonnet 4",
				model: "claude-sonnet-4-20250514",
			}),
		],
	},
};

export const SavesProviderKey: Story = {
	args: {
		onSave: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const apiKeyInput = await canvas.findByLabelText("API Key");
		await userEvent.type(apiKeyInput, "sk-test-key");
		await userEvent.click(canvas.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(args.onSave).toHaveBeenCalledWith("prov-1", "sk-test-key");
		});
	},
};

export const RemovesProviderKey: Story = {
	args: {
		providerItems: createProviderItems([
			createProvider({
				provider_id: "prov-1",
				provider: "openai",
				display_name: "OpenAI",
				has_user_api_key: true,
			}),
		]),
		onRemove: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const removeButton = await canvas.findByRole("button", { name: "Remove" });
		await userEvent.click(removeButton);

		const body = within(canvasElement.ownerDocument.body);
		await waitFor(() =>
			expect(body.getByText("Remove API key?")).toBeVisible(),
		);
		const dialog = await body.findByRole("dialog");
		await userEvent.click(
			within(dialog).getByRole("button", { name: "Remove" }),
		);

		await waitFor(() => {
			expect(args.onRemove).toHaveBeenCalledWith("prov-1");
		});
	},
};

export const ClearsMaskedApiKeyOnFocus: Story = {
	args: {
		providerItems: createProviderItems([
			createProvider({
				provider_id: "prov-1",
				provider: "openai",
				display_name: "OpenAI",
				has_user_api_key: true,
			}),
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const apiKeyInput = await canvas.findByLabelText("API Key");
		await expect(apiKeyInput).toHaveValue("••••••••••••••••");
		await userEvent.click(apiKeyInput);
		await expect(apiKeyInput).toHaveValue("");
	},
};

export const ShowsProviderStatuses: Story = {
	args: {
		providerItems: createProviderItems([
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
		]),
		models: [
			createModel({
				id: "model-openai-1",
				provider: "openai",
				display_name: "GPT-4o",
				model: "gpt-4o",
			}),
			createModel({
				id: "model-anthropic-1",
				provider: "anthropic",
				display_name: "Claude Sonnet 4",
				model: "claude-sonnet-4-20250514",
			}),
			createModel({
				id: "model-google-1",
				provider: "google",
				display_name: "Gemini 2.5 Pro",
				model: "gemini-2.5-pro",
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("Key saved")).toBeVisible();
		await expect(canvas.getByText("Using shared key")).toBeVisible();
		await expect(canvas.getByText("No key")).toBeVisible();
		await expect(
			canvas.getByText(
				"The shared deployment key is being used. Add a personal key to use your own.",
			),
		).toBeVisible();
		await expect(
			canvas.getByText("You must add a personal API key to use this provider."),
		).toBeVisible();
	},
};

export const WithError: Story = {
	args: {
		error: new Error("Failed to load provider configurations"),
	},
};
