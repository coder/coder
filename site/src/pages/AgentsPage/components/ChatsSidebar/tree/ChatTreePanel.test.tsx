import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Chat, ChatTreeResponse } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import {
	MockChatTreeChild,
	MockChatTreeGrandchild,
	MockChatTreeResponse,
	MockChatTreeRoot,
	MockChatTreeSibling,
} from "#/testHelpers/chatEntities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes, { DEFAULT_THEME } from "#/theme";
import {
	AGENT_CHAT_STATUS_ORDER,
	DEFAULT_AGENT_SIDEBAR_FILTERS,
} from "../../../utils/agentSidebarFilters";
import { ChatTreePanel, type ChatTreePanelData } from "./ChatTreePanel";
import { loadChatTreeExpansion } from "./chatTreeExpansion";

const organization = { id: "org-1", displayName: "Acme" };

const LocationProbe: FC = () => {
	const location = useLocation();
	return <div data-testid="location">{location.pathname}</div>;
};

const Wrapper: FC<PropsWithChildren<{ initialPath?: string }>> = ({
	children,
	initialPath = "/agents",
}) => (
	<ThemeOverride theme={themes[DEFAULT_THEME]}>
		<TooltipProvider>
			<QueryClientProvider client={createTestQueryClient()}>
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

const renderPanel = (
	props: Partial<Parameters<typeof ChatTreePanel>[0]> = {},
	options: { initialPath?: string } = {},
) => {
	const handlers = {
		onArchiveAgent: vi.fn(),
		onUnarchiveAgent: vi.fn(),
		onArchiveAndDeleteWorkspace: vi.fn(),
		onPinAgent: vi.fn(),
		onUnpinAgent: vi.fn(),
		onOpenRenameDialog: vi.fn(),
		onSidebarFiltersChange: vi.fn(),
	};
	const data = props.data ?? buildData();
	const view = render(
		<Wrapper initialPath={options.initialPath}>
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
		</Wrapper>,
	);
	return { ...view, handlers, data };
};

const treeitem = (name: string | RegExp) =>
	screen.getByRole("treeitem", { name });

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
		expect(document.activeElement?.getAttribute("aria-label")).toBe(
			"Open actions for Root",
		);
		await user.tab();
		expect(document.activeElement).toBe(document.body);
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

	it("persists the collapse of a root and expansion of a chat", async () => {
		renderPanel();
		const user = userEvent.setup();
		await user.click(
			within(
				screen.getByTestId(`chat-tree-row-${MockChatTreeRoot.id}`),
			).getByRole("button", { name: "Collapse" }),
		);
		expect(
			loadChatTreeExpansion().collapsedTopLevel.has(MockChatTreeRoot.id),
		).toBe(true);
	});
});

describe("ChatTreePanel actions", () => {
	it("requests a child chat from the actions menu", async () => {
		const { data } = renderPanel();
		const user = userEvent.setup();
		await user.click(
			screen.getByRole("button", {
				name: `Open actions for ${MockChatTreeChild.title}`,
			}),
		);
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
		await user.click(
			screen.getByRole("button", { name: "Open actions for Deepest" }),
		);
		const item = screen.getByRole("menuitem", { name: /New chat here/ });
		expect(item.getAttribute("aria-disabled")).toBe("true");
		await user.click(item);
		expect(data.onCreateChildChat).not.toHaveBeenCalled();
	});

	it("archives a leaf immediately and confirms before archiving a subtree", async () => {
		const { handlers } = renderPanel({ data: buildData(idleTree) });
		const user = userEvent.setup();
		await user.click(
			screen.getByRole("button", {
				name: `Open actions for ${MockChatTreeSibling.title}`,
			}),
		);
		await user.click(screen.getByRole("menuitem", { name: "Archive agent" }));
		expect(handlers.onArchiveAgent).toHaveBeenCalledWith(
			MockChatTreeSibling.id,
		);

		await user.click(
			screen.getByRole("button", {
				name: `Open actions for ${MockChatTreeChild.title}`,
			}),
		);
		await user.click(screen.getByRole("menuitem", { name: "Archive agent" }));
		expect(handlers.onArchiveAgent).toHaveBeenCalledTimes(1);
		const dialog = screen.getByRole("dialog", { name: "Archive chat" });
		expect(dialog.textContent).toContain("1 chat beneath it");
		await user.click(within(dialog).getByRole("button", { name: "Archive" }));
		expect(handlers.onArchiveAgent).toHaveBeenCalledWith(MockChatTreeChild.id);
	});

	it("moves focus to the next row after an archived row disappears", async () => {
		const { rerender, handlers } = renderPanel({ data: buildData(idleTree) });
		const user = userEvent.setup();
		await user.click(
			screen.getByRole("button", {
				name: `Open actions for ${MockChatTreeSibling.title}`,
			}),
		);
		await user.click(screen.getByRole("menuitem", { name: "Archive agent" }));
		expect(handlers.onArchiveAgent).toHaveBeenCalled();

		rerender(
			<Wrapper>
				<ChatTreePanel
					data={buildData({
						root_chat_id: MockChatTreeRoot.id,
						chats: [
							MockChatTreeRoot,
							MockChatTreeChild,
							MockChatTreeGrandchild,
						],
					})}
					sidebarFilters={DEFAULT_AGENT_SIDEBAR_FILTERS}
					activeChatId={undefined}
					modelConfigs={[]}
					isLoadingModelConfigs={false}
					chatErrorReasons={{}}
					isArchiving={false}
					archivingChatId={null}
					{...handlers}
				/>
			</Wrapper>,
		);
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
		expect(screen.getByRole("status").textContent).toBe("1 chat matches");
		expect(
			screen.getAllByRole("treeitem").map((item) => item.textContent),
		).not.toContainEqual(expect.stringContaining(MockChatTreeSibling.title));

		await user.click(treeitem(/Fix flaky login test/));
		await user.keyboard("{ArrowLeft}");
		expect(activeName()).toContain("Root");
		expect(loadChatTreeExpansion().expanded.has(MockChatTreeChild.id)).toBe(
			false,
		);
		expect(AGENT_CHAT_STATUS_ORDER).toContain("unread");
	});

	it("hides the owned tree when only shared chats are selected", () => {
		renderPanel({
			sidebarFilters: {
				...DEFAULT_AGENT_SIDEBAR_FILTERS,
				sources: ["shared_with_me"],
			},
		});
		expect(screen.queryAllByRole("treeitem")).toHaveLength(0);
	});
});
