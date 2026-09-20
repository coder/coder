import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { chatEntityKey } from "#/api/queries/chats";
import type { Chat, ChatTreeResponse } from "#/api/typesGenerated";
import {
	MockChat,
	MockChatTreeChild,
	MockChatTreeGrandchild,
	MockChatTreeResponse,
	MockChatTreeRoot,
	MockChatTreeSibling,
	MockChatTreeSubagent,
} from "#/testHelpers/chatEntities";
import { MockChatModel } from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { DEFAULT_AGENT_SIDEBAR_FILTERS } from "../../../utils/agentSidebarFilters";
import { ChatsSidebar } from "../ChatsSidebar";
import type { ChatTreePanelData } from "./ChatTreePanel";
import { CHAT_TREE_EXPANSION_STORAGE_KEY } from "./chatTreeExpansion";

const organization = {
	id: MockDefaultOrganization.id,
	displayName: MockDefaultOrganization.display_name,
};

const agentsRouting = [
	{ path: "/agents/:agentId", useStoryElement: true },
	{ path: "/agents", useStoryElement: true },
] satisfies [
	{ path: string; useStoryElement: boolean },
	...{ path: string; useStoryElement: boolean }[],
];

const treeData = (
	response: ChatTreeResponse,
	overrides: Partial<ChatTreePanelData> = {},
): ChatTreePanelData => ({
	organizations: [organization],
	responsesByOrganization: new Map([[organization.id, response]]),
	sharedChats: [],
	onCreateChildChat: fn(),
	...overrides,
});

const treeChats = MockChatTreeResponse.chats;

const meta: Meta<typeof ChatsSidebar> = {
	title: "pages/AgentsPage/ChatsSidebar/ChatTreePanel",
	component: ChatsSidebar,
	decorators: [
		withAuthProvider,
		withDashboardProvider,
		(Story) => {
			// Expansion is persisted per browser; start every story from the
			// defaults so screenshots do not depend on the previous story.
			localStorage.removeItem(CHAT_TREE_EXPANSION_STORAGE_KEY);
			return (
				<div style={{ height: 640, width: 320 }}>
					<Story />
				</div>
			);
		},
	],
	args: {
		chats: treeChats,
		chatErrorReasons: {},
		modelConfigs: [MockChatModel],
		onArchiveAgent: fn(),
		onUnarchiveAgent: fn(),
		onArchiveAndDeleteWorkspace: fn(),
		onPinAgent: fn(),
		onUnpinAgent: fn(),
		onReorderPinnedAgent: fn(),
		onRenameTitle: fn(() => Promise.resolve()),
		onBeforeNewAgent: fn(),
		isSearchDialogOpen: false,
		onSearchDialogOpenChange: fn(),
		isCreating: false,
		currentUserId: MockUserOwner.id,
		sidebarFilters: DEFAULT_AGENT_SIDEBAR_FILTERS,
		onSidebarFiltersChange: fn(),
		treeData: treeData(MockChatTreeResponse),
	},
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		experiments: ["chat-tree"],
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
};

export default meta;
type Story = StoryObj<typeof ChatsSidebar>;

export const RootOnly: Story = {
	args: {
		chats: [MockChatTreeRoot],
		treeData: treeData({
			root_chat_id: MockChatTreeRoot.id,
			chats: [MockChatTreeRoot],
		}),
	},
};

export const NoRootAvailable: Story = {
	args: {
		chats: [],
		treeData: treeData({ root_chat_id: null, chats: [] }),
	},
};

export const Collapsed: Story = {};

export const Expanded: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			within(
				canvas.getByTestId(`chat-tree-row-${MockChatTreeChild.id}`),
			).getByRole("button", { name: "Expand" }),
		);
	},
};

export const ActiveChatHighlighted: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents/:agentId",
				pathParams: { agentId: MockChatTreeGrandchild.id },
			},
			routing: agentsRouting,
		}),
	},
};

const statusRows: Chat[] = [
	MockChatTreeRoot,
	{
		...MockChatTreeChild,
		title: "Waiting for approval",
		status: "requires_action",
		child_chat_count: 0,
	},
	{ ...MockChatTreeSibling, title: "Running build", status: "running" },
	{
		...MockChatTreeGrandchild,
		id: "chat-tree-error",
		parent_chat_id: MockChatTreeRoot.id,
		depth: 2,
		title: "Failed deploy",
		status: "error",
		last_error: { message: "Provider timeout", retryable: false },
	},
	{
		...MockChatTreeGrandchild,
		id: "chat-tree-unread",
		parent_chat_id: MockChatTreeRoot.id,
		depth: 2,
		title: "Unread result",
		status: "waiting",
		has_unread: true,
	},
	{
		...MockChatTreeGrandchild,
		id: "chat-tree-interrupting",
		parent_chat_id: MockChatTreeRoot.id,
		depth: 2,
		title: "Stopping agent",
		status: "interrupting",
	},
];

export const StatusIndicators: Story = {
	args: {
		chats: statusRows,
		treeData: treeData({
			root_chat_id: MockChatTreeRoot.id,
			chats: statusRows,
		}),
	},
};

const deepChats: Chat[] = [
	MockChatTreeRoot,
	{ ...MockChatTreeChild, title: "Level 2" },
	{ ...MockChatTreeGrandchild, title: "Level 3", child_chat_count: 1 },
	{
		...MockChatTreeGrandchild,
		id: "level-4",
		parent_chat_id: MockChatTreeGrandchild.id,
		depth: 4,
		title: "Level 4",
		child_chat_count: 1,
	},
	{
		...MockChatTreeGrandchild,
		id: "level-5",
		parent_chat_id: "level-4",
		depth: 5,
		title: "Level 5 (limit)",
		child_chat_count: 0,
	},
];

const expandAll = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	for (let index = 0; index < 4; index++) {
		const buttons = canvas.queryAllByRole("button", { name: "Expand" });
		if (buttons.length === 0) {
			return;
		}
		await userEvent.click(buttons[0]);
	}
};

export const DeepTree: Story = {
	args: {
		chats: deepChats,
		treeData: treeData({ root_chat_id: MockChatTreeRoot.id, chats: deepChats }),
	},
	play: async ({ canvasElement }) => {
		await expandAll(canvasElement);
	},
};

export const DepthLimitDisablesCreate: Story = {
	...DeepTree,
	play: async ({ canvasElement }) => {
		await expandAll(canvasElement);
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Open actions for Level 5 (limit)",
			}),
		);
	},
};

export const ArchiveConfirmation: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: `Open actions for ${MockChatTreeChild.title}`,
			}),
		);
		await userEvent.click(
			within(document.body).getByRole("menuitem", { name: "Archive agent" }),
		);
	},
};

const pinnedChats: Chat[] = treeChats.map((chat) =>
	chat.id === MockChatTreeSibling.id ? { ...chat, pin_order: 1 } : chat,
);

export const PinnedSection: Story = {
	args: {
		chats: pinnedChats,
		treeData: treeData({
			root_chat_id: MockChatTreeRoot.id,
			chats: pinnedChats,
		}),
	},
};

const sharedChat: Chat = {
	...MockChat,
	id: "shared-1",
	title: "Shared: incident review",
	owner_id: "someone-else",
	shared: true,
};

export const SharedSection: Story = {
	args: {
		chats: [...treeChats, sharedChat],
		sidebarFilters: {
			...DEFAULT_AGENT_SIDEBAR_FILTERS,
			sources: ["created_by_me", "shared_with_me"],
		},
		treeData: treeData(MockChatTreeResponse, { sharedChats: [sharedChat] }),
	},
};

const archivedChats: Chat[] = [
	MockChatTreeRoot,
	{ ...MockChatTreeGrandchild, archived: true },
	{ ...MockChatTreeSibling, archived: true, status: "waiting" },
];

export const ArchivedForest: Story = {
	args: {
		chats: archivedChats,
		sidebarFilters: {
			...DEFAULT_AGENT_SIDEBAR_FILTERS,
			archiveStatus: "archived",
		},
		treeData: treeData({
			root_chat_id: MockChatTreeRoot.id,
			chats: archivedChats,
		}),
	},
};

const unreadChats: Chat[] = treeChats.map((chat) =>
	chat.id === MockChatTreeGrandchild.id ? { ...chat, has_unread: true } : chat,
);

export const UnreadFilterDimsAncestors: Story = {
	args: {
		chats: unreadChats,
		sidebarFilters: {
			...DEFAULT_AGENT_SIDEBAR_FILTERS,
			chatStatuses: ["unread"],
		},
		treeData: treeData({
			root_chat_id: MockChatTreeRoot.id,
			chats: unreadChats,
		}),
	},
};

export const SubagentsShown: Story = {
	parameters: {
		queries: [
			{
				key: chatEntityKey(MockChatTreeChild.id),
				data: { ...MockChatTreeChild, children: [MockChatTreeSubagent] },
			},
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: `Open actions for ${MockChatTreeChild.title}`,
			}),
		);
		await userEvent.click(
			within(document.body).getByRole("menuitem", { name: "Show subagents" }),
		);
	},
};

const organization2Root: Chat = {
	...MockChatTreeRoot,
	id: "org2-root",
	organization_id: MockOrganization2.id,
	child_chat_count: 1,
};
const organization2Child: Chat = {
	...MockChatTreeChild,
	id: "org2-child",
	organization_id: MockOrganization2.id,
	parent_chat_id: organization2Root.id,
	title: "Rotate signing keys",
	child_chat_count: 0,
};

export const MultipleOrganizations: Story = {
	args: {
		chats: [...treeChats, organization2Root, organization2Child],
		treeData: {
			organizations: [
				organization,
				{
					id: MockOrganization2.id,
					displayName: MockOrganization2.display_name,
				},
			],
			responsesByOrganization: new Map([
				[organization.id, MockChatTreeResponse],
				[
					MockOrganization2.id,
					{
						root_chat_id: organization2Root.id,
						chats: [organization2Root, organization2Child],
					},
				],
			]),
			sharedChats: [],
			onCreateChildChat: fn(),
		},
	},
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
};

export const KeyboardFocusRing: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		canvas.getByRole("treeitem", { name: /Root/ }).focus();
		await userEvent.keyboard("{ArrowDown}");
	},
};

export const Mobile: Story = {
	parameters: {
		viewport: { defaultViewport: "mobile1" },
	},
	decorators: [
		(Story) => (
			<div style={{ height: 560, width: 360 }}>
				<Story />
			</div>
		),
	],
};
