import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC, PropsWithChildren } from "react";
import { type QueryClient, QueryClientProvider } from "react-query";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { chatEntityKey } from "#/api/queries/chats";
import type { Chat, ChatTreeResponse } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import {
	MockChatTreeChild,
	MockChatTreeGrandchild,
	MockChatTreeResponse,
	MockChatTreeRoot,
	MockChatTreeSibling,
	MockChatTreeSubagent,
} from "#/testHelpers/chatEntities";
import { MockOrganization2 } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes, { DEFAULT_THEME } from "#/theme";
import { DEFAULT_AGENT_SIDEBAR_FILTERS } from "../../../utils/agentSidebarFilters";
import { ChatTreePanel, type ChatTreePanelData } from "./ChatTreePanel";
import { loadChatTreeExpansion } from "./chatTreeExpansion";
import { organizationTreeNodeId } from "./chatTreeModel";

const organization = { id: "org-1", displayName: "Acme" };
const organization2 = {
	id: MockOrganization2.id,
	displayName: MockOrganization2.display_name,
};

const LocationProbe: FC = () => {
	const location = useLocation();
	return <div data-testid="location">{location.pathname}</div>;
};

const Wrapper: FC<
	PropsWithChildren<{ initialPath?: string; queryClient?: QueryClient }>
> = ({ children, initialPath = "/agents", queryClient }) => (
	<ThemeOverride theme={themes[DEFAULT_THEME]}>
		<TooltipProvider>
			<QueryClientProvider client={queryClient ?? createTestQueryClient()}>
				<MemoryRouter initialEntries={[initialPath]}>
					<Routes>
						<Route
							path="/agents/:agentId?"
							element={
								<>
									{children}
									<LocationProbe />
								</>
							}
						/>
					</Routes>
				</MemoryRouter>
			</QueryClientProvider>
		</TooltipProvider>
	</ThemeOverride>
);

// The sibling fixture is running, which blocks archive; these tests need
// an idle leaf.
const idleTree: ChatTreeResponse = {
	...MockChatTreeResponse,
	chats: MockChatTreeResponse.chats.map((chat) =>
		chat.id === MockChatTreeSibling.id ? { ...chat, status: "waiting" } : chat,
	),
};

const buildData = (
	response: ChatTreeResponse = MockChatTreeResponse,
	overrides: Partial<ChatTreePanelData> = {},
): ChatTreePanelData => ({
	organizations: [organization],
	responsesByOrganization: new Map([[organization.id, response]]),
	sharedChats: [],
	onCreateChildChat: vi.fn(),
	...overrides,
});

const organization2Root: Chat = {
	...MockChatTreeRoot,
	id: "org2-root",
	organization_id: organization2.id,
};

const twoOrganizationsData = (): ChatTreePanelData => ({
	organizations: [organization, organization2],
	responsesByOrganization: new Map([
		[organization.id, MockChatTreeResponse],
		[
			organization2.id,
			{ root_chat_id: organization2Root.id, chats: [organization2Root] },
		],
	]),
	sharedChats: [],
	onCreateChildChat: vi.fn(),
});

type PanelProps = Parameters<typeof ChatTreePanel>[0];

const defaultHandlers = () => ({
	onArchiveAgent: vi.fn(),
	onUnarchiveAgent: vi.fn(),
	onArchiveAndDeleteWorkspace: vi.fn(),
	onPinAgent: vi.fn(),
	onUnpinAgent: vi.fn(),
	onOpenRenameDialog: vi.fn(),
	onSidebarFiltersChange: vi.fn(),
});

const panelElement = (
	props: Partial<PanelProps>,
	handlers: ReturnType<typeof defaultHandlers>,
	data: ChatTreePanelData,
) => (
	<ChatTreePanel
		data={data}
		sidebarFilters={DEFAULT_AGENT_SIDEBAR_FILTERS}
		activeChatId={undefined}
		modelConfigs={[]}
		isLoadingModelConfigs={false}
		chatErrorReasons={{}}
		isArchiving={false}
		archivingChatId={null}
		{...handlers}
		{...props}
	/>
);

const renderPanel = (
	props: Partial<PanelProps> = {},
	options: { initialPath?: string; queryClient?: QueryClient } = {},
) => {
	const handlers = defaultHandlers();
	const data = props.data ?? buildData();
	const view = render(
		<Wrapper
			initialPath={options.initialPath}
			queryClient={options.queryClient}
		>
			{panelElement(props, handlers, data)}
		</Wrapper>,
	);
	const rerenderPanel = (nextProps: Partial<PanelProps>) =>
		view.rerender(
			<Wrapper
				initialPath={options.initialPath}
				queryClient={options.queryClient}
			>
				{panelElement(nextProps, handlers, nextProps.data ?? data)}
			</Wrapper>,
		);
	return { ...view, handlers, data, rerenderPanel };
};

const treeitem = (name: string | RegExp) =>
	screen.getByRole("treeitem", { name });

const openActions = async (
	user: ReturnType<typeof userEvent.setup>,
	title: string,
) => {
	await user.click(
		screen.getByRole("button", { name: `Open actions for ${title}` }),
	);
};

const activeName = () =>
	document.activeElement instanceof HTMLElement
		? document.activeElement.textContent
		: null;

beforeEach(() => {
	localStorage.clear();
});

describe("ChatTreePanel keyboard navigation", () => {
	it("moves focus with arrows, Home and End over visible rows", async () => {
		renderPanel();
		const user = userEvent.setup();
		await user.tab();
		expect(activeName()).toContain("Root");

		await user.keyboard("{ArrowDown}");
		expect(activeName()).toContain(MockChatTreeChild.title);
		await user.keyboard("{ArrowDown}");
		expect(activeName()).toContain(MockChatTreeSibling.title);
		await user.keyboard("{End}");
		expect(activeName()).toContain(MockChatTreeSibling.title);
		await user.keyboard("{Home}");
		expect(activeName()).toContain("Root");
	});

	it("expands with Right, moves into the child, and collapses with Left", async () => {
		renderPanel();
		const user = userEvent.setup();
		await user.tab();
		await user.keyboard("{ArrowDown}");
		await user.keyboard("{ArrowRight}");
		expect(loadChatTreeExpansion().expanded.has(MockChatTreeChild.id)).toBe(
			true,
		);
		await user.keyboard("{ArrowRight}");
		expect(activeName()).toContain(MockChatTreeGrandchild.title);
		await user.keyboard("{ArrowLeft}");
		expect(activeName()).toContain(MockChatTreeChild.title);
		await user.keyboard("{ArrowLeft}");
		expect(loadChatTreeExpansion().expanded.has(MockChatTreeChild.id)).toBe(
			false,
		);
	});

	it("expands the collapsed siblings of the focused row on *", async () => {
		renderPanel();
		const user = userEvent.setup();
		await user.click(treeitem(/Migrate billing service/));
		await user.keyboard("*");
		expect(loadChatTreeExpansion().expanded.has(MockChatTreeChild.id)).toBe(
			true,
		);
		await user.keyboard("{ArrowUp}");
		expect(activeName()).toContain(MockChatTreeGrandchild.title);
	});

	it("type-ahead focuses the next row whose title starts with the character", async () => {
		renderPanel();
		const user = userEvent.setup();
		await user.tab();
		await user.keyboard("m");
		expect(activeName()).toContain(MockChatTreeSibling.title);
	});

	it("keeps a single tab stop: treeitem, its actions, then out", async () => {
		renderPanel();
		const user = userEvent.setup();
		await user.tab();
		expect(activeName()).toContain("Root");
		await user.tab();
		expect(document.activeElement).toBe(
			screen.getByRole("button", { name: "Open actions for Root" }),
		);
		await user.tab();
		expect(document.activeElement).toBe(document.body);
	});

	it("moves the tab stop to the active chat after navigation", async () => {
		const { rerenderPanel } = renderPanel();
		const user = userEvent.setup();
		await user.tab();
		expect(activeName()).toContain("Root");
		await user.tab();
		await user.tab();
		expect(document.activeElement).toBe(document.body);

		rerenderPanel({ activeChatId: MockChatTreeSibling.id });
		await user.tab();
		expect(activeName()).toContain(MockChatTreeSibling.title);
		await user.tab();
		expect(document.activeElement).toBe(
			screen.getByRole("button", {
				name: `Open actions for ${MockChatTreeSibling.title}`,
			}),
		);
	});

	it("opens the focused chat with Enter", async () => {
		renderPanel();
		const user = userEvent.setup();
		await user.tab();
		await user.keyboard("{ArrowDown}");
		await user.keyboard("{Enter}");
		expect(screen.getByTestId("location").textContent).toBe(
			`/agents/${MockChatTreeChild.id}`,
		);
	});

	it("toggles an organization node with Enter and Space", async () => {
		renderPanel({ data: twoOrganizationsData() });
		const user = userEvent.setup();
		const organizationNodeId = organizationTreeNodeId(organization.id);
		await user.tab();
		expect(activeName()).toContain(organization.displayName);
		await user.keyboard("{Enter}");
		expect(
			loadChatTreeExpansion().collapsedTopLevel.has(organizationNodeId),
		).toBe(true);
		await user.keyboard(" ");
		expect(
			loadChatTreeExpansion().collapsedTopLevel.has(organizationNodeId),
		).toBe(false);
	});
});

describe("ChatTreePanel expansion", () => {
	it("expands the collapsed ancestors of the active chat", () => {
		renderPanel(
			{ activeChatId: MockChatTreeGrandchild.id },
			{ initialPath: `/agents/${MockChatTreeGrandchild.id}` },
		);
		expect(loadChatTreeExpansion().expanded.has(MockChatTreeChild.id)).toBe(
			true,
		);
	});

	it("expands and collapses from the actions menu", async () => {
		renderPanel();
		const user = userEvent.setup();
		await openActions(user, MockChatTreeChild.title);
		await user.click(screen.getByRole("menuitem", { name: "Expand" }));
		expect(loadChatTreeExpansion().expanded.has(MockChatTreeChild.id)).toBe(
			true,
		);
		await openActions(user, "Root");
		await user.click(screen.getByRole("menuitem", { name: "Collapse" }));
		expect(
			loadChatTreeExpansion().collapsedTopLevel.has(MockChatTreeRoot.id),
		).toBe(true);
	});

	it("shows subagents from the actions menu as navigable rows", async () => {
		const queryClient = createTestQueryClient();
		// The test client garbage-collects unobserved data at once and would
		// refetch stale data from a server that does not exist here.
		queryClient.setQueryDefaults(chatEntityKey(MockChatTreeChild.id), {
			gcTime: Number.POSITIVE_INFINITY,
			staleTime: Number.POSITIVE_INFINITY,
		});
		queryClient.setQueryData(chatEntityKey(MockChatTreeChild.id), {
			...MockChatTreeChild,
			children: [MockChatTreeSubagent],
		});
		renderPanel({}, { queryClient });
		const user = userEvent.setup();
		await openActions(user, MockChatTreeChild.title);
		await user.click(screen.getByRole("menuitem", { name: "Show subagents" }));
		expect(loadChatTreeExpansion().subagents.has(MockChatTreeChild.id)).toBe(
			true,
		);
		await user.click(treeitem(/Fix flaky login test/));
		await user.keyboard("{ArrowDown}");
		expect(activeName()).toContain(MockChatTreeGrandchild.title);
		await user.keyboard("{ArrowDown}");
		expect(activeName()).toContain(MockChatTreeSubagent.title);
	});
});

describe("ChatTreePanel actions", () => {
	it("requests a child chat from the actions menu", async () => {
		const { data } = renderPanel();
		const user = userEvent.setup();
		await openActions(user, MockChatTreeChild.title);
		await user.click(screen.getByRole("menuitem", { name: "New chat here" }));
		expect(data.onCreateChildChat).toHaveBeenCalledWith(MockChatTreeChild);
	});

	it("keeps the depth-limited item focusable but inert", async () => {
		const deep: Chat = {
			...MockChatTreeGrandchild,
			id: "deep",
			title: "Deepest",
			depth: 5,
		};
		const { data } = renderPanel({
			data: buildData({
				root_chat_id: MockChatTreeRoot.id,
				chats: [MockChatTreeRoot, deep],
			}),
		});
		const user = userEvent.setup();
		await openActions(user, "Deepest");
		const item = screen.getByRole("menuitem", { name: /New chat here/ });
		await user.click(item);
		expect(data.onCreateChildChat).not.toHaveBeenCalled();
		expect(document.activeElement).toBe(item);
	});

	it("archives a leaf and a subtree without a panel dialog", async () => {
		const { handlers } = renderPanel({ data: buildData(idleTree) });
		const user = userEvent.setup();
		await openActions(user, MockChatTreeSibling.title);
		await user.click(screen.getByRole("menuitem", { name: "Archive agent" }));
		expect(handlers.onArchiveAgent).toHaveBeenCalledWith(
			MockChatTreeSibling.id,
		);

		await openActions(user, MockChatTreeChild.title);
		await user.click(screen.getByRole("menuitem", { name: "Archive agent" }));
		expect(handlers.onArchiveAgent).toHaveBeenCalledWith(MockChatTreeChild.id);
	});

	it("moves focus to the next row after an archived row disappears", async () => {
		const { rerenderPanel, handlers } = renderPanel({
			data: buildData(idleTree),
		});
		const user = userEvent.setup();
		await openActions(user, MockChatTreeSibling.title);
		await user.click(screen.getByRole("menuitem", { name: "Archive agent" }));
		expect(handlers.onArchiveAgent).toHaveBeenCalled();

		rerenderPanel({
			data: buildData({
				root_chat_id: MockChatTreeRoot.id,
				chats: [MockChatTreeRoot, MockChatTreeChild, MockChatTreeGrandchild],
			}),
		});
		expect(activeName()).toContain(MockChatTreeChild.title);
	});
});

describe("ChatTreePanel filters", () => {
	it("keeps ancestors of unread matches and does not collapse them on Left", async () => {
		const unreadGrandchild: Chat = {
			...MockChatTreeGrandchild,
			has_unread: true,
		};
		renderPanel({
			data: buildData({
				root_chat_id: MockChatTreeRoot.id,
				chats: [
					MockChatTreeRoot,
					MockChatTreeChild,
					MockChatTreeSibling,
					unreadGrandchild,
				],
			}),
			sidebarFilters: {
				...DEFAULT_AGENT_SIDEBAR_FILTERS,
				chatStatuses: ["unread"],
			},
		});
		const user = userEvent.setup();
		await user.click(treeitem(/Fix flaky login test/));
		await user.keyboard("{ArrowLeft}");
		expect(activeName()).toContain("Root");
		expect(loadChatTreeExpansion().expanded.has(MockChatTreeChild.id)).toBe(
			false,
		);
	});
});
