import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, within } from "storybook/test";
import { API } from "#/api/api";
import {
	chatModelConfigs,
	userChatProviderConfigsKey,
} from "#/api/queries/chats";
import type {
	ChatModelConfig,
	UserChatProviderConfig,
} from "#/api/typesGenerated";
import { MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "#/testHelpers/storybook";
import ProvidersPage from "./ProvidersPage";

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

const meta = {
	title: "pages/UserSettingsPage/ProvidersPage",
	component: ProvidersPage,
	parameters: {
		user: MockUserOwner,
	},
	decorators: [withToaster, withAuthProvider, withDashboardProvider],
} satisfies Meta<typeof ProvidersPage>;

export default meta;
type Story = StoryObj<typeof ProvidersPage>;

export const Default: Story = {
	parameters: {
		queries: [
			{
				key: userChatProviderConfigsKey,
				data: [
					createProvider({
						provider_id: "prov-1",
						provider: "openai",
						display_name: "OpenAI",
					}),
				],
			},
			{
				key: chatModelConfigs().queryKey,
				data: [
					createModel({
						id: "model-1",
						provider: "openai",
						display_name: "GPT-4o",
						model: "gpt-4o",
					}),
				],
			},
		],
	},
};

export const WithSavedKey: Story = {
	parameters: {
		queries: [
			{
				key: userChatProviderConfigsKey,
				data: [
					createProvider({
						provider_id: "prov-1",
						provider: "openai",
						display_name: "OpenAI",
						has_user_api_key: true,
						has_central_api_key_fallback: true,
					}),
				],
			},
			{
				key: chatModelConfigs().queryKey,
				data: [
					createModel({
						id: "model-1",
						provider: "openai",
						display_name: "GPT-4o",
						model: "gpt-4o",
					}),
				],
			},
		],
	},
};

export const MasksApiKeyInput: Story = {
	parameters: {
		queries: [
			{
				key: userChatProviderConfigsKey,
				data: [
					createProvider({
						provider_id: "prov-1",
						provider: "openai",
						display_name: "OpenAI",
						has_user_api_key: true,
					}),
				],
			},
			{
				key: chatModelConfigs().queryKey,
				data: [],
			},
		],
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
	parameters: {
		queries: [
			{
				key: userChatProviderConfigsKey,
				data: [
					createProvider({
						provider_id: "prov-1",
						provider: "anthropic",
						display_name: "Anthropic",
						has_central_api_key_fallback: true,
					}),
				],
			},
			{
				key: chatModelConfigs().queryKey,
				data: [
					createModel({
						id: "model-1",
						provider: "anthropic",
						display_name: "Claude Sonnet 4",
						model: "claude-sonnet-4-20250514",
					}),
				],
			},
		],
	},
};

export const MultipleProviders: Story = {
	parameters: {
		queries: [
			{
				key: userChatProviderConfigsKey,
				data: [
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
			},
			{
				key: chatModelConfigs().queryKey,
				data: [
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
		],
	},
};

export const Empty: Story = {
	parameters: {
		queries: [
			{
				key: userChatProviderConfigsKey,
				data: [],
			},
			{
				key: chatModelConfigs().queryKey,
				data: [],
			},
		],
	},
};

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getUserChatProviderConfigs").mockImplementation(
			async () => {
				return new Promise<UserChatProviderConfig[]>(() => {
					return;
				});
			},
		);
		spyOn(API.experimental, "getChatModelConfigs").mockResolvedValue([]);
	},
};
