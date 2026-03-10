import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
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
import {
	expect,
	fn,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
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
	},
	beforeEach: () => {
		spyOn(API, "getChatSystemPrompt").mockResolvedValue({
			system_prompt: "",
		});
		spyOn(API, "updateChatSystemPrompt").mockResolvedValue();
		spyOn(API, "getUserChatCustomPrompt").mockResolvedValue({
			custom_prompt: "",
		});
		spyOn(API, "updateUserChatCustomPrompt").mockResolvedValue({
			custom_prompt: "",
		});
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
	},
	beforeEach: () => {
		spyOn(API, "getChatSystemPrompt").mockResolvedValue({
			system_prompt: "You are a helpful coding assistant.",
		});
	},
};

/** Admin with model config permissions sees Providers/Models tabs. */
export const AdminFull: Story = {
	args: {
		canSetSystemPrompt: true,
		canManageChatModelConfigs: true,
	},
	parameters: { queries: chatQueries },
	beforeEach: () => {
		spyOn(API, "getChatSystemPrompt").mockResolvedValue({
			system_prompt: "Follow company coding standards.",
		});
	},
};

/** Verifies that typing and saving the system prompt calls the API. */
export const SavesBehaviorPromptAndRestores: Story = {
	args: {
		canSetSystemPrompt: true,
	},
	play: async () => {
		const dialog = await screen.findByRole("dialog");

		// Find the System Instructions textarea by its unique placeholder.
		const textareas = await within(dialog).findAllByPlaceholderText(
			"Additional behavior, style, and tone preferences for all users",
		);
		const textarea = textareas[0];

		await userEvent.type(textarea, "You are a focused coding assistant.");

		// Click the Save button inside the System Instructions form.
		// There are multiple Save buttons (one per form), so grab all and
		// pick the last one which belongs to the system prompt section.
		const saveButtons = within(dialog).getAllByRole("button", { name: "Save" });
		await userEvent.click(saveButtons[saveButtons.length - 1]);

		await waitFor(() => {
			expect(API.updateChatSystemPrompt).toHaveBeenCalledWith({
				system_prompt: "You are a focused coding assistant.",
			});
		});
	},
};
