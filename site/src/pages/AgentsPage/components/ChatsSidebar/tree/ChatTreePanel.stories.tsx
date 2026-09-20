import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
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

/** Default sidebar frame; a story overrides it through `parameters.frame`. */
const DEFAULT_FRAME = { width: 320, height: 640 };

const meta: Meta<typeof ChatsSidebar> = {
	title: "pages/AgentsPage/ChatsSidebar/ChatTreePanel",
	component: ChatsSidebar,
	decorators: [
		withAuthProvider,
		withDashboardProvider,
		(Story, { parameters }) => {
			// Expansion is persisted per browser; start every story from the
			// defaults so screenshots do not depend on the previous story.
			localStorage.removeItem(CHAT_TREE_EXPANSION_STORAGE_KEY);
			const frame: typeof DEFAULT_FRAME = parameters.frame ?? DEFAULT_FRAME;
			return (
				<div style={{ height: frame.height, width: frame.width }}>
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

// The chevron is hidden from assistive technology, so stories expand rows
// the way a keyboard user does: focus the treeitem and press Right.
const expandTreeitem = async (treeitem: HTMLElement) => {
	treeitem.focus();
	await userEvent.keyboard("{ArrowRight}");
};

export const Expanded: Story = {
	play: async ({ canvasElement }) => {
		await expandTreeitem(
			within(canvasElement).getByRole("treeitem", {
				name: new RegExp(MockChatTreeChild.title),
			}),
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
		const collapsed = canvas.queryAllByRole("treeitem", { expanded: false });
		if (collapsed.length === 0) {
			return;
		}
		await expandTreeitem(collapsed[0]);
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

export const ContextMenuOpen: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.pointer({
			keys: "[MouseRight]",
			target: within(canvasElement).getByTestId(
				`chat-tree-row-${MockChatTreeChild.id}`,
			),
		});
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

// The pinned sibling does not match the unread filter, so only the pinned
// unread child remains in the pinned section and reordering is disabled.
const filteredPinnedChats: Chat[] = treeChats.map((chat) =>
	chat.id === MockChatTreeSibling.id
		? { ...chat, pin_order: 1 }
		: chat.id === MockChatTreeChild.id
			? { ...chat, pin_order: 2, has_unread: true }
			: chat,
);

export const PinnedSectionFiltered: Story = {
	args: {
		chats: filteredPinnedChats,
		sidebarFilters: {
			...DEFAULT_AGENT_SIDEBAR_FILTERS,
			chatStatuses: ["unread"],
		},
		treeData: treeData({
			root_chat_id: MockChatTreeRoot.id,
			chats: filteredPinnedChats,
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

export const FilterWithoutMatches: Story = {
	args: {
		sidebarFilters: {
			...DEFAULT_AGENT_SIDEBAR_FILTERS,
			chatStatuses: ["unread"],
		},
	},
};

const showSubagents = async (canvasElement: HTMLElement) => {
	await userEvent.click(
		within(canvasElement).getByRole("button", {
			name: `Open actions for ${MockChatTreeChild.title}`,
		}),
	);
	await userEvent.click(
		within(document.body).getByRole("menuitem", { name: "Show subagents" }),
	);
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
		await showSubagents(canvasElement);
	},
};

// No cached entity, so the toggle starts a fetch that never settles.
export const SubagentsLoading: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChat").mockImplementation(
			() => new Promise(() => {}),
		);
	},
	play: async ({ canvasElement }) => {
		await showSubagents(canvasElement);
	},
};

export const SubagentsLoadFailed: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChat").mockRejectedValue(
			new Error("Request failed"),
		);
	},
	play: async ({ canvasElement }) => {
		await showSubagents(canvasElement);
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

export const CollapsedOrganization: Story = {
	...MultipleOrganizations,
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("treeitem", {
				name: MockOrganization2.display_name,
			}),
		);
	},
};

export const KeyboardFocusRing: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		canvas.getByRole("treeitem", { name: /Root/ }).focus();
		await userEvent.keyboard("{ArrowDown}");
	},
};

// Pixel captures with a mouse-capable browser, so the hover-revealed
// controls render as on desktop; touch (hover: none) emulation is not
// available here.
export const Mobile: Story = {
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		frame: { width: 360, height: 560 },
	},
};
