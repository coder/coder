import type { Meta, StoryObj } from "@storybook/react-vite";
import type {
	ChatModelConfig,
	ChatModelsResponse,
	ChatProviderConfig,
} from "api/typesGenerated";
import { expect, fn, userEvent, within } from "storybook/test";
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
	{ key: ["chat-provider-configs"], data: mockProviderConfigs },
	{ key: ["chat-model-configs"], data: mockModelConfigs },
	{ key: ["chat-models"], data: mockChatModels },
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
		isDisabled: false,
	},
};

export default meta;
type Story = StoryObj<typeof ConfigureAgentsDialog>;

export const SystemPromptOnly: Story = {
	args: {
		canSetSystemPrompt: true,
		canManageChatModelConfigs: false,
		systemPromptDraft: "You are a helpful coding assistant.",
	},
};

export const ModelConfigOnly: Story = {
	args: {
		canSetSystemPrompt: false,
		canManageChatModelConfigs: true,
	},
	parameters: { queries: chatQueries },
};

export const BothEnabled: Story = {
	args: {
		canSetSystemPrompt: true,
		canManageChatModelConfigs: true,
		systemPromptDraft: "Follow company coding standards.",
	},
	parameters: { queries: chatQueries },
};

export const EditAndSaveSystemPrompt: Story = {
	args: {
		canSetSystemPrompt: true,
		canManageChatModelConfigs: false,
		systemPromptDraft: "",
		onSystemPromptDraftChange: fn(),
		onSaveSystemPrompt: fn(),
		isSystemPromptDirty: true,
		isDisabled: false,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		// The Behavior section should be visible by default when it is
		// the only available section.
		const textarea = canvas.getByPlaceholderText(
			"Optional. Set deployment-wide instructions for all new chats.",
		);
		expect(textarea).toBeVisible();

		// Type into the textarea. The component is controlled, so the
		// onChange handler should be called.
		await userEvent.type(textarea, "Be concise.");
		expect(args.onSystemPromptDraftChange).toHaveBeenCalled();

		// Because isSystemPromptDirty is true, the Save button should
		// be enabled.
		const saveButton = canvas.getByRole("button", { name: "Save" });
		expect(saveButton).toBeEnabled();

		await userEvent.click(saveButton);
		expect(args.onSaveSystemPrompt).toHaveBeenCalled();
	},
};
