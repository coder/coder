import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { createChatStore } from "#/pages/AgentsPage/components/ChatConversation/chatStore";
import { FIXTURE_NOW } from "#/pages/AgentsPage/components/ChatConversation/storyFixtures";
import type { ModelSelectorOption } from "#/pages/AgentsPage/components/ChatElements";
import { CoderAssistantPanel } from "./CoderAssistantPanel";

const CHAT_ID = "coder-assistant-panel-stories";

const defaultModelOptions: ModelSelectorOption[] = [
	{
		id: "model-config-1",
		provider: "anthropic",
		model: "claude-sonnet-4-5",
		displayName: "Claude Sonnet 4.5",
	},
];

const buildMessage = (
	id: number,
	role: TypesGen.ChatMessageRole,
	content: TypesGen.ChatMessagePart[],
): TypesGen.ChatMessage => ({
	id,
	chat_id: CHAT_ID,
	created_at: new Date(FIXTURE_NOW - (10 - id) * 60_000).toISOString(),
	role,
	content,
});

const buildSeededStore = () => {
	const store = createChatStore();
	store.replaceMessages([
		buildMessage(1, "user", [{ type: "text", text: "What can you do?" }]),
		buildMessage(2, "assistant", [
			{
				type: "text",
				text: "I can help you create workspaces, build templates, and answer questions about your Coder deployment.",
			},
		]),
	]);
	return store;
};

const meta: Meta<typeof CoderAssistantPanel> = {
	title: "components/CoderAssistantPanel",
	component: CoderAssistantPanel,
	// The panel is fixed to the bottom-right of the viewport, so
	// fullscreen layout keeps it visible in the canvas.
	parameters: {
		layout: "fullscreen",
	},
	args: {
		open: true,
		onClose: fn(),
		onNewChat: fn(),
		onDisable: fn(),
		onSendMessage: fn(),
		isThinking: false,
		isSendPending: false,
		isStreaming: false,
		onInterrupt: fn(),
		isInterruptPending: false,
		chatId: null,
		store: createChatStore(),
		persistedError: undefined,
		modelOptions: defaultModelOptions,
		selectedModel: defaultModelOptions[0].id,
		onModelChange: fn(),
		hasModelOptions: true,
		modelSelectorPlaceholder: "Select model",
		isModelCatalogLoading: false,
	},
};

export default meta;
type Story = StoryObj<typeof CoderAssistantPanel>;

export const EmptyState: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText("Hi, I'm your Coder Assistant"),
		).toBeInTheDocument();
		const chip = canvas.getByRole("button", { name: "What can you do?" });
		expect(chip).toBeEnabled();
	},
};

export const EmptyStateDisabled: Story = {
	args: {
		modelOptions: [],
		selectedModel: "",
		hasModelOptions: false,
		modelSelectorPlaceholder: "No models configured",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const chip = canvas.getByRole("button", { name: "What can you do?" });
		expect(chip).toBeDisabled();
	},
};

export const WithMessages: Story = {
	args: {
		chatId: CHAT_ID,
		store: buildSeededStore(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("What can you do?")).toBeInTheDocument();
		expect(
			canvas.getByText(/create workspaces, build templates/),
		).toBeInTheDocument();
	},
};
