import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	chatModelConfigsKey,
	chatModelsKey,
	chatProviderConfigsKey,
} from "api/queries/chats";
import type {
	ChatModelConfig,
	ChatModelsResponse,
	ChatProviderConfig,
} from "api/typesGenerated";
import { fn } from "storybook/test";
import { ConfigureAgentsDialog } from "./ConfigureAgentsDialog";

// Pre-seeded query data so that ChatModelAdminPanel renders
// without hitting a real backend.
const mockProviderConfigs: ChatProviderConfig[] = [
	{
		id: "provider-1",
		provider: "openai",
		display_name: "OpenAI",
		enabled: true,
		has_api_key: true,
		base_url: "https://api.openai.com/v1",
		source: "database",
		created_at: "2025-01-01T00:00:00Z",
		updated_at: "2025-01-01T00:00:00Z",
	},
];

const mockModelConfigs: ChatModelConfig[] = [
	{
		id: "model-cfg-1",
		provider: "openai",
		model: "gpt-4o",
		display_name: "GPT-4o",
		enabled: true,
		is_default: false,
		context_limit: 128000,
		compression_threshold: 80000,
		created_at: "2025-01-01T00:00:00Z",
		updated_at: "2025-01-01T00:00:00Z",
	},
];

const mockChatModels: ChatModelsResponse = {
	providers: [
		{
			provider: "openai",
			available: true,
			models: [
				{
					id: "openai:gpt-4o",
					provider: "openai",
					model: "gpt-4o",
					display_name: "GPT-4o",
				},
			],
		},
	],
};

const chatQueries = [
	{ key: chatProviderConfigsKey, data: mockProviderConfigs },
	{ key: chatModelConfigsKey, data: mockModelConfigs },
	{ key: chatModelsKey, data: mockChatModels },
];

const meta: Meta<typeof ConfigureAgentsDialog> = {
	title: "pages/AgentsPage/ConfigureAgentsDialog",
	component: ConfigureAgentsDialog,
	args: {
		open: true,
		onOpenChange: fn(),
		canManageChatModelConfigs: false,
		canSetSystemPrompt: false,
		systemPromptDraft: "",
		onSystemPromptDraftChange: fn(),
		onSaveSystemPrompt: fn(),
		isSystemPromptDirty: false,
		saveSystemPromptError: false,
		userPromptDraft: "",
		onUserPromptDraftChange: fn(),
		onSaveUserPrompt: fn(),
		isUserPromptDirty: false,
		saveUserPromptError: false,
		isDisabled: false,
	},
};

export default meta;
type Story = StoryObj<typeof ConfigureAgentsDialog>;

/** Regular user sees only the Personal Prompt section. */
export const UserOnly: Story = {};

/** Admin sees Personal Prompt + System Prompt in the same Prompts tab. */
export const AdminPrompts: Story = {
	args: {
		canSetSystemPrompt: true,
		systemPromptDraft: "You are a helpful coding assistant.",
	},
};

/** Admin with model config permissions sees Providers/Models tabs. */
export const AdminFull: Story = {
	args: {
		canSetSystemPrompt: true,
		canManageChatModelConfigs: true,
		systemPromptDraft: "Follow company coding standards.",
	},
	parameters: { queries: chatQueries },
};
