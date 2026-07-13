import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { assistantChats } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	ChatTreeContext,
	type ChatTreeContextValue,
} from "../tree/ChatTreeContext";
import { buildChatTree } from "../tree/chatTree";
import { AssistantChatsSection } from "./AssistantChatsSection";

const buildAssistantChat = (overrides: Partial<Chat>): Chat => ({
	...MockChat,
	labels: { "coder-agent": "true" },
	...overrides,
});

const assistantChatList: Chat[] = [
	buildAssistantChat({ id: "assistant-chat-1", title: "Create a workspace" }),
	buildAssistantChat({ id: "assistant-chat-2", title: "Build a template" }),
	buildAssistantChat({ id: "assistant-chat-3", title: "Debug my agent" }),
];

const buildTreeContextValue = (
	chats: readonly Chat[],
): ChatTreeContextValue => ({
	chatTree: buildChatTree(chats),
	chatById: new Map(chats.map((chat) => [chat.id, chat])),
	visibleChatIDs: new Set(chats.map((chat) => chat.id)),
	normalizedSearch: "",
	expandedById: {},
	modelOptions: [],
	modelConfigs: [],
	chatErrorReasons: {},
	activeChatId: undefined,
	isArchiving: false,
	archivingChatId: null,
	toggleExpanded: fn(),
	onArchiveAgent: fn(),
	onUnarchiveAgent: fn(),
	onArchiveAndDeleteWorkspace: fn(),
	onPinAgent: fn(),
	onUnpinAgent: fn(),
});

const withChatTree =
	(chats: readonly Chat[]): Decorator =>
	(Story) => (
		<ChatTreeContext value={buildTreeContextValue(chats)}>
			<Story />
		</ChatTreeContext>
	);

// The component persists its expanded state in localStorage, so reset
// it before each story to keep the default (collapsed) deterministic.
const withCollapsedPreferenceReset: Decorator = (Story) => {
	localStorage.removeItem("coder_agent_history_expanded");
	return <Story />;
};

const meta: Meta<typeof AssistantChatsSection> = {
	title: "pages/AgentsPage/ChatsSidebar/AssistantChatsSection",
	component: AssistantChatsSection,
	decorators: [withCollapsedPreferenceReset, withChatTree([])],
};

export default meta;
type Story = StoryObj<typeof AssistantChatsSection>;

const expandSection = async (canvas: ReturnType<typeof within>) => {
	await userEvent.click(
		canvas.getByRole("button", { name: "Expand Assistant section" }),
	);
};

export const Collapsed: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button", {
			name: "Expand Assistant section",
		});
		expect(toggle).toHaveAttribute("aria-expanded", "false");
	},
};

export const ExpandedEmpty: Story = {
	parameters: {
		queries: [
			{
				key: assistantChats().queryKey,
				data: [],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expandSection(canvas);
		expect(
			canvas.getByText("No assistant conversations yet"),
		).toBeInTheDocument();
	},
};

export const ExpandedWithChats: Story = {
	decorators: [withChatTree(assistantChatList)],
	parameters: {
		queries: [
			{
				key: assistantChats().queryKey,
				data: assistantChatList,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expandSection(canvas);
		expect(canvas.getByText("Create a workspace")).toBeInTheDocument();
		expect(canvas.getByText("Build a template")).toBeInTheDocument();
		expect(canvas.getByText("Debug my agent")).toBeInTheDocument();
	},
};
