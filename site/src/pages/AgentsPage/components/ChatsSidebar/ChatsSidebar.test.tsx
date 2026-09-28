import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import type { Chat } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockChatModel } from "#/testHelpers/chatModels";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
	MockUserOwner,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes, { DEFAULT_THEME } from "#/theme";
import {
	AGENT_CHAT_STATUS_ORDER,
	type AgentSidebarFilters,
	DEFAULT_AGENT_SIDEBAR_FILTERS,
} from "../../utils/agentSidebarFilters";
import { ChatsSidebar } from "./ChatsSidebar";

// ---- IntersectionObserver mock ----

type IOCallback = (entries: Array<{ isIntersecting: boolean }>) => void;
let observerCallback: IOCallback | null = null;
let observeCount = 0;

class MockIntersectionObserver {
	observe = vi.fn(() => {
		observeCount++;
	});
	disconnect = vi.fn();
	unobserve = vi.fn();

	constructor(cb: IOCallback) {
		observerCallback = cb;
	}
}

// ---- Auth mock ----

vi.mock("#/hooks/useAuthenticated", async () => {
	return {
		useAuthenticated: () => ({
			user: MockUserOwner,
			permissions: {},
			signOut: vi.fn(),
		}),
	};
});

// ---- Helpers ----

const oneWeekAgo = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString();

const buildChat = (overrides: Partial<Chat> = {}): Chat => ({
	...MockChat,
	id: "chat-default",
	last_model_config_id: "model-1",
	created_at: oneWeekAgo,
	updated_at: oneWeekAgo,
	...overrides,
});

const dashboardValue = {
	entitlements: MockEntitlements,
	experiments: [] as TypesGen.Experiment[],
	appearance: MockAppearanceConfig,
	buildInfo: MockBuildInfo,
	organizations: [MockDefaultOrganization],
	showOrganizations: false,
	canViewOrganizationSettings: false,
};

type WrapperProps = {
	children: React.ReactNode;
	initialPath?: string;
};

const Wrapper: React.FC<WrapperProps> = ({
	children,
	initialPath = "/agents",
}) => {
	const queryClient = createTestQueryClient();
	return (
		<QueryClientProvider client={queryClient}>
			<ThemeOverride theme={themes[DEFAULT_THEME]}>
				<TooltipProvider>
					<MemoryRouter initialEntries={[initialPath]}>
						<DashboardContext.Provider value={dashboardValue}>
							{children}
						</DashboardContext.Provider>
					</MemoryRouter>
				</TooltipProvider>
			</ThemeOverride>
		</QueryClientProvider>
	);
};

const defaultSidebarFilters: AgentSidebarFilters = {
	archiveStatus: "active",
	groupBy: "date",
	prStatuses: [],
	chatStatuses: AGENT_CHAT_STATUS_ORDER,
	unread: false,
	sources: ["created_by_me", "shared_with_me"],
};

const defaultProps: React.ComponentProps<typeof ChatsSidebar> = {
	chats: [buildChat({ id: "chat-1", title: "Chat One" })],
	chatErrorReasons: {},
	modelConfigs: [],
	onArchiveAgent: vi.fn(),
	onUnarchiveAgent: vi.fn(),
	onArchiveAndDeleteWorkspace: vi.fn(),
	onPinAgent: vi.fn(),
	onUnpinAgent: vi.fn(),
	onMarkChatRead: vi.fn(),
	onMarkChatUnread: vi.fn(),
	onRenameTitle: vi.fn(async () => {}),
	onBeforeNewAgent: vi.fn(),
	isSearchDialogOpen: false,
	onSearchDialogOpenChange: vi.fn(),
	isCreating: false,
	sidebarFilters: defaultSidebarFilters,
	onSidebarFiltersChange: vi.fn(),
	currentUserId: MockUserOwner.id,
};

// ---- Tests ----

describe("ChatsSidebar section switcher", () => {
	const LocationProbe: React.FC = () => {
		const location = useLocation();
		return <div data-testid="location-pathname">{location.pathname}</div>;
	};

	it.each([
		["Workspaces", "/workspaces"],
		["Templates", "/templates"],
		["Agents", "/agents"],
	])("navigates to %s", async (label, pathname) => {
		const user = userEvent.setup();

		render(
			<Wrapper initialPath="/agents/chat-1">
				<ChatsSidebar {...defaultProps} />
				<LocationProbe />
			</Wrapper>,
		);

		await user.click(screen.getByRole("button", { name: "Agents" }));
		await user.click(await screen.findByRole("menuitem", { name: label }));

		await waitFor(() => {
			expect(screen.getByTestId("location-pathname").textContent).toBe(
				pathname,
			);
		});
	});
});

describe("ChatsSidebar sections", () => {
	it("renders unpinned shared chats in Shared with you before date sections", () => {
		render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					chats={[
						buildChat({
							id: "pinned-shared-chat",
							title: "Pinned shared chat",
							shared: true,
							pin_order: 1,
						}),
						buildChat({
							id: "shared-chat",
							title: "Shared chat",
							owner_id: "sharing-user-id",
							shared: true,
						}),
						buildChat({
							id: "owned-shared-chat",
							title: "Owned shared chat",
							shared: true,
							updated_at: new Date().toISOString(),
						}),
						buildChat({
							id: "owned-chat",
							title: "Owned chat",
							updated_at: new Date().toISOString(),
						}),
					]}
				/>
			</Wrapper>,
		);

		const pinnedSection = screen.getByTestId("agents-section-toggle-Pinned");
		const pinnedSharedNode = screen.getByTestId(
			"agents-tree-node-pinned-shared-chat",
		);
		const sharedSection = screen.getByTestId(
			"agents-section-toggle-Shared-with-you",
		);
		const sharedNode = screen.getByTestId("agents-tree-node-shared-chat");
		const todaySection = screen.getByTestId("agents-section-toggle-Today");
		const ownedNode = screen.getByTestId("agents-tree-node-owned-chat");

		expect(pinnedSection).toHaveTextContent("Pinned (1)");
		expect(sharedSection).toHaveTextContent("Shared with you (1)");
		expect(todaySection).toHaveTextContent("Today (2)");
		expect(
			pinnedSection.compareDocumentPosition(pinnedSharedNode) &
				Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
		expect(
			pinnedSharedNode.compareDocumentPosition(sharedSection) &
				Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
		expect(
			sharedSection.compareDocumentPosition(sharedNode) &
				Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
		expect(
			sharedNode.compareDocumentPosition(todaySection) &
				Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
		expect(
			todaySection.compareDocumentPosition(ownedNode) &
				Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
	});
});

type MenuUser = ReturnType<typeof userEvent.setup>;

const openFilterMenu = async (user: MenuUser) => {
	await user.click(screen.getByRole("button", { name: "Filter agents" }));
};

// Every element reports a zero-size rect in jsdom, so Radix's pointer grace
// area closes a submenu as soon as user-event moves the pointer into it.
// Keyboard navigation drives the same selection path, and the real pointer
// path is covered by the FilterPopover stories.
const focusMenuItem = async (
	user: MenuUser,
	role: "menuitem" | "menuitemcheckbox" | "menuitemradio",
	name: string | RegExp,
) => {
	const item = await screen.findByRole(role, { name });
	for (let step = 0; step < 16 && document.activeElement !== item; step++) {
		await user.keyboard("{ArrowDown}");
	}
	expect(item).toHaveFocus();
};

const toggleSubmenuOption = async (
	user: MenuUser,
	submenu: string | RegExp,
	name: string,
	role: "menuitemcheckbox" | "menuitemradio" = "menuitemcheckbox",
) => {
	await openFilterMenu(user);
	await focusMenuItem(user, "menuitem", submenu);
	await user.keyboard("{ArrowRight}");
	await focusMenuItem(user, role, name);
	await user.keyboard("{Enter}");
	await user.keyboard("{Escape}{Escape}");
};

describe("ChatsSidebar filters", () => {
	const ControlledSidebar: React.FC<{
		initialFilters?: AgentSidebarFilters;
		onChange: (filters: AgentSidebarFilters) => void;
	}> = ({ initialFilters = defaultSidebarFilters, onChange }) => {
		const [filters, setFilters] = useState(initialFilters);
		return (
			<ChatsSidebar
				{...defaultProps}
				sidebarFilters={filters}
				onSidebarFiltersChange={(next) => {
					setFilters(next);
					onChange(next);
				}}
			/>
		);
	};
	it("selects archived from the state radio group", async () => {
		const user = userEvent.setup();
		const onSidebarFiltersChange = vi.fn();

		render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					sidebarFilters={defaultSidebarFilters}
					onSidebarFiltersChange={onSidebarFiltersChange}
				/>
			</Wrapper>,
		);

		await user.click(screen.getByRole("button", { name: "Filter agents" }));
		await user.click(
			await screen.findByRole("menuitemradio", { name: "Archived" }),
		);

		expect(onSidebarFiltersChange).toHaveBeenCalledWith({
			...defaultSidebarFilters,
			archiveStatus: "archived",
		});
	});

	it("clears only result filters when applied filters return no agents", async () => {
		const user = userEvent.setup();
		const onSidebarFiltersChange = vi.fn();
		const sidebarFilters: AgentSidebarFilters = {
			...defaultSidebarFilters,
			archiveStatus: "archived",
			groupBy: "chat_status",
			prStatuses: ["draft"],
			chatStatuses: ["running"],
			sources: ["shared_with_me"],
		};

		render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					chats={[]}
					sidebarFilters={sidebarFilters}
					onSidebarFiltersChange={onSidebarFiltersChange}
				/>
			</Wrapper>,
		);

		expect(
			screen.getByRole("button", { name: "Filter agents" }),
		).toBeInTheDocument();
		expect(
			screen.getByText("No agents match these filters"),
		).toBeInTheDocument();

		await user.click(screen.getByRole("button", { name: "Clear filters" }));

		expect(onSidebarFiltersChange).toHaveBeenCalledWith({
			...sidebarFilters,
			prStatuses: [],
			chatStatuses: defaultSidebarFilters.chatStatuses,
			sources: defaultSidebarFilters.sources,
		});
	});

	it("selects Mine, Shared with me, and All owners", async () => {
		const user = userEvent.setup();
		const onSidebarFiltersChange = vi.fn();

		render(
			<Wrapper>
				<ControlledSidebar onChange={onSidebarFiltersChange} />
			</Wrapper>,
		);

		await toggleSubmenuOption(user, /^Owner/, "Mine", "menuitemradio");

		expect(onSidebarFiltersChange).toHaveBeenLastCalledWith({
			...defaultSidebarFilters,
			sources: ["created_by_me"],
		});

		await toggleSubmenuOption(
			user,
			/^Owner/,
			"Shared with me",
			"menuitemradio",
		);
		expect(onSidebarFiltersChange).toHaveBeenLastCalledWith({
			...defaultSidebarFilters,
			sources: ["shared_with_me"],
		});

		await toggleSubmenuOption(user, /^Owner/, "All", "menuitemradio");
		expect(onSidebarFiltersChange).toHaveBeenLastCalledWith(
			defaultSidebarFilters,
		);
	});

	it("clearing the last status restores all statuses without changing owner", async () => {
		const user = userEvent.setup();
		const onSidebarFiltersChange = vi.fn();
		const sidebarFilters: AgentSidebarFilters = {
			...defaultSidebarFilters,
			chatStatuses: ["running"],
			sources: ["shared_with_me"],
		};

		render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					sidebarFilters={sidebarFilters}
					onSidebarFiltersChange={onSidebarFiltersChange}
				/>
			</Wrapper>,
		);

		await toggleSubmenuOption(user, /^Filter by/, "Status: Working");

		expect(onSidebarFiltersChange).toHaveBeenCalledWith({
			...sidebarFilters,
			chatStatuses: defaultSidebarFilters.chatStatuses,
		});
	});

	it("applies the unread checkbox", async () => {
		const user = userEvent.setup();
		const onSidebarFiltersChange = vi.fn();

		render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					sidebarFilters={defaultSidebarFilters}
					onSidebarFiltersChange={onSidebarFiltersChange}
				/>
			</Wrapper>,
		);

		await toggleSubmenuOption(user, /^Filter by/, "Unread");

		expect(onSidebarFiltersChange).toHaveBeenCalledWith({
			...defaultSidebarFilters,
			unread: true,
		});
	});

	it.each([
		["PR: draft", "draft"],
		["PR: open", "open"],
		["PR: merged", "merged"],
		["PR: closed", "closed"],
		["No PR", "none"],
	] as const)("filters by %s", async (label, status) => {
		const user = userEvent.setup();
		const onSidebarFiltersChange = vi.fn();
		render(
			<Wrapper>
				<ControlledSidebar onChange={onSidebarFiltersChange} />
			</Wrapper>,
		);

		await toggleSubmenuOption(user, /^Filter by/, label);
		expect(onSidebarFiltersChange).toHaveBeenCalledWith({
			...defaultSidebarFilters,
			prStatuses: [status],
		});
	});

	it.each([
		["Status: Requires action", "requires_action"],
		["Status: Error", "error"],
		["Status: Working", "running"],
		["Status: Idle", "waiting"],
	] as const)("narrows all statuses to %s", async (label, status) => {
		const user = userEvent.setup();
		const onSidebarFiltersChange = vi.fn();
		render(
			<Wrapper>
				<ControlledSidebar onChange={onSidebarFiltersChange} />
			</Wrapper>,
		);

		await toggleSubmenuOption(user, /^Filter by/, label);
		expect(onSidebarFiltersChange).toHaveBeenCalledWith({
			...defaultSidebarFilters,
			chatStatuses: [status],
		});
	});

	it("adds successive statuses, then restores all when the last is cleared", async () => {
		const user = userEvent.setup();
		const onSidebarFiltersChange = vi.fn();
		render(
			<Wrapper>
				<ControlledSidebar onChange={onSidebarFiltersChange} />
			</Wrapper>,
		);

		await toggleSubmenuOption(user, /^Filter by/, "Status: Error");
		await toggleSubmenuOption(user, /^Filter by/, "Status: Working");
		expect(onSidebarFiltersChange).toHaveBeenLastCalledWith({
			...defaultSidebarFilters,
			chatStatuses: ["error", "running"],
		});
		await toggleSubmenuOption(user, /^Filter by/, "Status: Error");
		expect(onSidebarFiltersChange).toHaveBeenLastCalledWith({
			...defaultSidebarFilters,
			chatStatuses: ["running"],
		});
		await toggleSubmenuOption(user, /^Filter by/, "Status: Working");
		expect(onSidebarFiltersChange).toHaveBeenLastCalledWith(
			defaultSidebarFilters,
		);
	});

	it("removes selected badges with the keyboard", async () => {
		const user = userEvent.setup();
		const onSidebarFiltersChange = vi.fn();
		render(
			<Wrapper>
				<ControlledSidebar
					initialFilters={{
						...defaultSidebarFilters,
						unread: true,
						prStatuses: ["draft"],
						chatStatuses: ["error"],
					}}
					onChange={onSidebarFiltersChange}
				/>
			</Wrapper>,
		);

		for (const [label, expected] of [
			["PR: draft", { unread: true, prStatuses: [], chatStatuses: ["error"] }],
			[
				"Status: Error",
				{ unread: true, prStatuses: [], chatStatuses: AGENT_CHAT_STATUS_ORDER },
			],
			[
				"Unread",
				{
					unread: false,
					prStatuses: [],
					chatStatuses: AGENT_CHAT_STATUS_ORDER,
				},
			],
		] as const) {
			await openFilterMenu(user);
			await focusMenuItem(user, "menuitem", `Remove ${label} filter`);
			await user.keyboard("{Enter}{Escape}");
			expect(onSidebarFiltersChange).toHaveBeenLastCalledWith({
				...defaultSidebarFilters,
				...expected,
			});
		}
	});

	it("changes grouping without changing filter selections", async () => {
		const user = userEvent.setup();
		const onSidebarFiltersChange = vi.fn();
		render(
			<Wrapper>
				<ControlledSidebar onChange={onSidebarFiltersChange} />
			</Wrapper>,
		);

		await toggleSubmenuOption(user, /^Grouped by/, "Status", "menuitemradio");
		expect(onSidebarFiltersChange).toHaveBeenLastCalledWith({
			...defaultSidebarFilters,
			groupBy: "chat_status",
		});
		await toggleSubmenuOption(user, /^Grouped by/, "Date", "menuitemradio");
		expect(onSidebarFiltersChange).toHaveBeenLastCalledWith(
			defaultSidebarFilters,
		);
	});

	it("resets every sidebar filter from the menu with the keyboard", async () => {
		const user = userEvent.setup();
		const onSidebarFiltersChange = vi.fn();

		render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					sidebarFilters={{
						...defaultSidebarFilters,
						archiveStatus: "archived",
						groupBy: "chat_status",
						prStatuses: ["draft"],
						chatStatuses: ["error"],
						unread: true,
						sources: ["created_by_me"],
					}}
					onSidebarFiltersChange={onSidebarFiltersChange}
				/>
			</Wrapper>,
		);

		await openFilterMenu(user);
		await focusMenuItem(user, "menuitem", "Reset to defaults");
		await user.keyboard("{Enter}");

		expect(onSidebarFiltersChange).toHaveBeenCalledWith(
			DEFAULT_AGENT_SIDEBAR_FILTERS,
		);
	});

	it("groups unpinned chats by chat status", () => {
		render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					chats={[
						buildChat({
							id: "attention-chat",
							title: "Needs action",
							status: "requires_action",
						}),
						buildChat({
							id: "error-chat",
							title: "Failed chat",
							status: "error",
						}),
						buildChat({
							id: "working-chat",
							title: "Working chat",
							status: "running",
						}),
						buildChat({
							id: "interrupting-chat",
							title: "Interrupting chat",
							status: "interrupting",
						}),
						buildChat({
							id: "idle-chat",
							title: "Idle chat",
							status: "waiting",
						}),
					]}
					sidebarFilters={{
						...defaultSidebarFilters,
						groupBy: "chat_status",
					}}
				/>
			</Wrapper>,
		);

		const attentionSection = screen.getByTestId(
			"agents-section-toggle-Requires-action",
		);
		const errorSection = screen.getByTestId("agents-section-toggle-Error");
		const workingSection = screen.getByTestId("agents-section-toggle-Working");
		const interruptingSection = screen.getByTestId(
			"agents-section-toggle-Interrupting",
		);
		const idleSection = screen.getByTestId("agents-section-toggle-Idle");
		const attentionNode = screen.getByTestId("agents-tree-node-attention-chat");
		const errorNode = screen.getByTestId("agents-tree-node-error-chat");
		const workingNode = screen.getByTestId("agents-tree-node-working-chat");
		const interruptingNode = screen.getByTestId(
			"agents-tree-node-interrupting-chat",
		);
		const idleNode = screen.getByTestId("agents-tree-node-idle-chat");

		expect(
			screen.queryByTestId("agents-section-toggle-Today"),
		).not.toBeInTheDocument();
		for (const [before, after] of [
			[attentionSection, attentionNode],
			[attentionNode, errorSection],
			[errorSection, errorNode],
			[errorNode, workingSection],
			[workingSection, workingNode],
			[workingNode, interruptingSection],
			[interruptingSection, interruptingNode],
			[interruptingNode, idleSection],
			[idleSection, idleNode],
		] as const) {
			expect(
				before.compareDocumentPosition(after) &
					Node.DOCUMENT_POSITION_FOLLOWING,
			).toBeTruthy();
		}
	});
});

describe("ChatsSidebar load-more behavior", () => {
	beforeEach(() => {
		observerCallback = null;
		observeCount = 0;
		vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("calls onLoadMore when the sentinel becomes visible", () => {
		const onLoadMore = vi.fn();
		render(
			<Wrapper>
				<ChatsSidebar {...defaultProps} hasNextPage onLoadMore={onLoadMore} />
			</Wrapper>,
		);

		act(() => {
			observerCallback?.([{ isIntersecting: true }]);
		});

		expect(onLoadMore).toHaveBeenCalledTimes(1);
	});

	it("does NOT call onLoadMore when isFetchingNextPage is true", () => {
		const onLoadMore = vi.fn();
		render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					hasNextPage
					onLoadMore={onLoadMore}
					isFetchingNextPage
				/>
			</Wrapper>,
		);

		act(() => {
			observerCallback?.([{ isIntersecting: true }]);
		});

		expect(onLoadMore).not.toHaveBeenCalled();
	});

	it("does NOT recreate the observer when re-rendered with a new onLoadMore reference", () => {
		const onLoadMore1 = vi.fn();
		const { rerender } = render(
			<Wrapper>
				<ChatsSidebar {...defaultProps} hasNextPage onLoadMore={onLoadMore1} />
			</Wrapper>,
		);

		const countAfterMount = observeCount;

		// Re-render with a brand-new function reference, which was the
		// original bug trigger.
		const onLoadMore2 = vi.fn();
		rerender(
			<Wrapper>
				<ChatsSidebar {...defaultProps} hasNextPage onLoadMore={onLoadMore2} />
			</Wrapper>,
		);

		// The observer should NOT have been torn down and recreated.
		expect(observeCount).toBe(countAfterMount);

		// The new callback should still be the one invoked.
		act(() => {
			observerCallback?.([{ isIntersecting: true }]);
		});
		expect(onLoadMore1).not.toHaveBeenCalled();
		expect(onLoadMore2).toHaveBeenCalledTimes(1);
	});

	it("does NOT spam onLoadMore across multiple re-renders", () => {
		const onLoadMore = vi.fn();
		const { rerender } = render(
			<Wrapper>
				<ChatsSidebar {...defaultProps} hasNextPage onLoadMore={onLoadMore} />
			</Wrapper>,
		);

		// Sentinel becomes visible once.
		act(() => {
			observerCallback?.([{ isIntersecting: true }]);
		});
		expect(onLoadMore).toHaveBeenCalledTimes(1);

		// Parent re-renders many times with new inline arrow callbacks
		// (the pattern that caused the original bug).
		for (let i = 0; i < 10; i++) {
			rerender(
				<Wrapper>
					<ChatsSidebar
						{...defaultProps}
						hasNextPage
						onLoadMore={() => onLoadMore()}
					/>
				</Wrapper>,
			);
		}

		// Re-renders alone should NOT trigger additional onLoadMore calls;
		// only a new IntersectionObserver entry should.
		expect(onLoadMore).toHaveBeenCalledTimes(1);
	});

	it("resumes loading after isFetchingNextPage goes from true to false", () => {
		const onLoadMore = vi.fn();
		const { rerender } = render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					hasNextPage
					onLoadMore={onLoadMore}
					isFetchingNextPage
				/>
			</Wrapper>,
		);

		// Blocked while fetching.
		act(() => {
			observerCallback?.([{ isIntersecting: true }]);
		});
		expect(onLoadMore).not.toHaveBeenCalled();

		// Fetch completes.
		rerender(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					hasNextPage
					onLoadMore={onLoadMore}
					isFetchingNextPage={false}
				/>
			</Wrapper>,
		);

		// Observer fires again while sentinel is still visible.
		act(() => {
			observerCallback?.([{ isIntersecting: true }]);
		});
		expect(onLoadMore).toHaveBeenCalledTimes(1);
	});

	it("recreates the observer when isFetchingNextPage transitions to false so visible sentinels re-trigger", () => {
		const onLoadMore = vi.fn();
		const { rerender } = render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					hasNextPage
					onLoadMore={onLoadMore}
					isFetchingNextPage={false}
				/>
			</Wrapper>,
		);

		const countAfterMount = observeCount;
		expect(countAfterMount).toBe(1);

		// Start fetching, observer is torn down.
		rerender(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					hasNextPage
					onLoadMore={onLoadMore}
					isFetchingNextPage
				/>
			</Wrapper>,
		);

		// Fetch completes, a fresh observer is created, firing
		// an initial entry that detects the still-visible sentinel.
		rerender(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					hasNextPage
					onLoadMore={onLoadMore}
					isFetchingNextPage={false}
				/>
			</Wrapper>,
		);

		expect(observeCount).toBe(countAfterMount + 1);
	});

	it("does NOT render the sentinel when hasNextPage is false", () => {
		const onLoadMore = vi.fn();
		render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					hasNextPage={false}
					onLoadMore={onLoadMore}
				/>
			</Wrapper>,
		);

		// No observer should have been created since the sentinel
		// is not rendered.
		expect(observeCount).toBe(0);
	});
});

describe("ChatsSidebar subtitles", () => {
	const modelConfigs: TypesGen.ChatModel[] = [
		{
			...MockChatModel,
			id: "model-1",
			model: "gpt-4o",
			display_name: "GPT-4o",
			created_at: oneWeekAgo,
			updated_at: oneWeekAgo,
		},
	];

	it("shows the last turn summary when present and no error exists", () => {
		render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					chats={[
						buildChat({
							id: "summary-chat",
							title: "Summary chat",
							last_turn_summary: "Updated the Terraform template",
						}),
					]}
					modelConfigs={modelConfigs}
				/>
			</Wrapper>,
		);

		expect(
			screen.getByText("Updated the Terraform template"),
		).toBeInTheDocument();
		expect(screen.queryByText("GPT-4o")).not.toBeInTheDocument();
	});

	it("shows the error when both error and last turn summary exist", () => {
		render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					chats={[
						buildChat({
							id: "summary-error-chat",
							title: "Summary error chat",
							status: "error",
							last_error: {
								message: "Workspace startup failed",
								retryable: false,
							},
							last_turn_summary: "Provisioned a workspace",
						}),
					]}
					modelConfigs={modelConfigs}
				/>
			</Wrapper>,
		);

		expect(screen.getByText("Workspace startup failed")).toBeInTheDocument();
		expect(
			screen.queryByText("Provisioned a workspace"),
		).not.toBeInTheDocument();
	});

	it("falls back to the model name when no last turn summary exists", () => {
		render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					chats={[
						buildChat({
							id: "model-fallback-chat",
							title: "Model fallback chat",
						}),
					]}
					modelConfigs={modelConfigs}
				/>
			</Wrapper>,
		);

		expect(screen.getByText("GPT-4o")).toBeInTheDocument();
	});

	it("falls back to the model name when the last turn summary is blank", () => {
		render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					chats={[
						buildChat({
							id: "blank-summary-chat",
							title: "Blank summary chat",
							last_turn_summary: "   ",
						}),
					]}
					modelConfigs={modelConfigs}
				/>
			</Wrapper>,
		);

		expect(screen.getByText("GPT-4o")).toBeInTheDocument();
	});
});

describe("ChatsSidebar read state actions", () => {
	const openActionsMenu = async (title: string) => {
		const user = userEvent.setup();
		await user.click(
			screen.getByRole("button", { name: `Open actions for ${title}` }),
		);
		return user;
	};

	it("marks a read chat as unread", async () => {
		const onMarkChatUnread = vi.fn();
		render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					chats={[
						buildChat({
							id: "read-chat",
							title: "Read chat",
							has_unread: false,
						}),
					]}
					onMarkChatUnread={onMarkChatUnread}
				/>
			</Wrapper>,
		);

		const user = await openActionsMenu("Read chat");
		await user.click(
			await screen.findByRole("menuitem", { name: "Mark as unread" }),
		);

		expect(onMarkChatUnread).toHaveBeenCalledWith("read-chat");
	});

	it("marks an unread chat as read", async () => {
		const onMarkChatRead = vi.fn();
		render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					chats={[
						buildChat({
							id: "unread-chat",
							title: "Unread chat",
							has_unread: true,
						}),
					]}
					onMarkChatRead={onMarkChatRead}
				/>
			</Wrapper>,
		);

		const user = await openActionsMenu("Unread chat");
		await user.click(
			await screen.findByRole("menuitem", { name: "Mark as read" }),
		);

		expect(onMarkChatRead).toHaveBeenCalledWith("unread-chat");
	});
});
