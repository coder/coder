import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
	MockUserOwner,
} from "testHelpers/entities";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import type * as TypesGen from "api/typesGenerated";
import type { Chat } from "api/typesGenerated";
import { ThemeOverride } from "contexts/ThemeProvider";
import { DashboardContext } from "modules/dashboard/DashboardProvider";
import type { FC, PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import themes, { DEFAULT_THEME } from "theme";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentsSidebar } from "./AgentsSidebar";

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

vi.mock("hooks", async () => {
	const actual = await vi.importActual("hooks");
	return {
		...actual,
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
	id: "chat-default",
	owner_id: "owner-1",
	title: "Agent",
	status: "completed",
	last_model_config_id: "model-1",
	created_at: oneWeekAgo,
	updated_at: oneWeekAgo,
	archived: false,
	last_error: null,
	mcp_server_ids: [],
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

const Wrapper: FC<PropsWithChildren> = ({ children }) => {
	const queryClient = new QueryClient({
		defaultOptions: {
			queries: { retry: false, refetchOnWindowFocus: false },
		},
	});
	return (
		<QueryClientProvider client={queryClient}>
			<ThemeOverride theme={themes[DEFAULT_THEME]}>
				<MemoryRouter initialEntries={["/agents"]}>
					<DashboardContext.Provider value={dashboardValue}>
						{children}
					</DashboardContext.Provider>
				</MemoryRouter>
			</ThemeOverride>
		</QueryClientProvider>
	);
};

const RouteWrapper: FC<
	PropsWithChildren<{ initialEntries: readonly string[] }>
> = ({ children, initialEntries }) => {
	const queryClient = new QueryClient({
		defaultOptions: {
			queries: { retry: false, refetchOnWindowFocus: false },
		},
	});
	return (
		<QueryClientProvider client={queryClient}>
			<ThemeOverride theme={themes[DEFAULT_THEME]}>
				<MemoryRouter initialEntries={[...initialEntries]}>
					<DashboardContext.Provider value={dashboardValue}>
						<Routes>
							<Route path="/agents" element={children} />
							<Route path="/agents/:agentId" element={children} />
						</Routes>
					</DashboardContext.Provider>
				</MemoryRouter>
			</ThemeOverride>
		</QueryClientProvider>
	);
};

const defaultProps: React.ComponentProps<typeof AgentsSidebar> = {
	chats: [buildChat({ id: "chat-1", title: "Chat One" })],
	chatErrorReasons: {},
	modelOptions: [],
	modelConfigs: [],
	onArchiveAgent: vi.fn(),
	onUnarchiveAgent: vi.fn(),
	onArchiveAndDeleteWorkspace: vi.fn(),
	onBeforeNewAgent: vi.fn(),
	isCreating: false,
	archivedFilter: "active" as const,
};

// ---- Tests ----

describe("AgentsSidebar load-more behavior", () => {
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
				<AgentsSidebar {...defaultProps} hasNextPage onLoadMore={onLoadMore} />
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
				<AgentsSidebar
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
				<AgentsSidebar {...defaultProps} hasNextPage onLoadMore={onLoadMore1} />
			</Wrapper>,
		);

		const countAfterMount = observeCount;

		// Re-render with a brand-new function reference, which was the
		// original bug trigger.
		const onLoadMore2 = vi.fn();
		rerender(
			<Wrapper>
				<AgentsSidebar {...defaultProps} hasNextPage onLoadMore={onLoadMore2} />
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
				<AgentsSidebar {...defaultProps} hasNextPage onLoadMore={onLoadMore} />
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
					<AgentsSidebar
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
				<AgentsSidebar
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
				<AgentsSidebar
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

	it("does NOT render the sentinel when hasNextPage is false", () => {
		const onLoadMore = vi.fn();
		render(
			<Wrapper>
				<AgentsSidebar
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

describe("AgentsSidebar tree behavior", () => {
	it("expands and collapses a parent branch", () => {
		const parentChat = buildChat({ id: "root-1", title: "Root Chat" });
		const childChat = buildChat({
			id: "child-1",
			title: "Child Chat",
			parent_chat_id: parentChat.id,
			root_chat_id: parentChat.id,
		});

		render(
			<Wrapper>
				<AgentsSidebar {...defaultProps} chats={[parentChat, childChat]} />
			</Wrapper>,
		);

		expect(screen.queryByText("Child Chat")).not.toBeInTheDocument();

		fireEvent.click(screen.getByTestId(`agents-tree-toggle-${parentChat.id}`));
		expect(screen.getByText("Child Chat")).toBeInTheDocument();

		fireEvent.click(screen.getByTestId(`agents-tree-toggle-${parentChat.id}`));
		expect(screen.queryByText("Child Chat")).not.toBeInTheDocument();
	});

	it("auto-expands ancestors for the active chat route", async () => {
		const rootChat = buildChat({ id: "root-1", title: "Root Chat" });
		const middleChat = buildChat({
			id: "middle-1",
			title: "Middle Chat",
			parent_chat_id: rootChat.id,
			root_chat_id: rootChat.id,
		});
		const leafChat = buildChat({
			id: "leaf-1",
			title: "Leaf Chat",
			parent_chat_id: middleChat.id,
			root_chat_id: rootChat.id,
		});

		render(
			<RouteWrapper initialEntries={[`/agents/${leafChat.id}`]}>
				<AgentsSidebar
					{...defaultProps}
					chats={[rootChat, middleChat, leafChat]}
				/>
			</RouteWrapper>,
		);

		await screen.findByText("Leaf Chat");

		expect(
			screen.getByTestId(`agents-tree-toggle-${rootChat.id}`),
		).toHaveAttribute("aria-expanded", "true");
		expect(
			screen.getByTestId(`agents-tree-toggle-${middleChat.id}`),
		).toHaveAttribute("aria-expanded", "true");
		expect(screen.getByText("Leaf Chat")).toBeInTheDocument();
	});

	it("renders every root chat title when no search is applied", () => {
		const rootChats = [
			buildChat({ id: "root-1", title: "Alpha Root" }),
			buildChat({ id: "root-2", title: "Beta Root" }),
			buildChat({ id: "root-3", title: "Gamma Root" }),
		];

		render(
			<Wrapper>
				<AgentsSidebar {...defaultProps} chats={rootChats} />
			</Wrapper>,
		);

		for (const chat of rootChats) {
			expect(screen.getByText(chat.title)).toBeInTheDocument();
		}
	});
});

describe("AgentsSidebar render-path regression", () => {
	it("preserves sibling root nodes when expanding a separate branch", () => {
		const rootOne = buildChat({ id: "root-1", title: "Root One" });
		const rootOneChild = buildChat({
			id: "child-1",
			title: "Root One Child",
			parent_chat_id: rootOne.id,
			root_chat_id: rootOne.id,
		});
		const rootTwo = buildChat({ id: "root-2", title: "Root Two" });
		const rootTwoChild = buildChat({
			id: "child-2",
			title: "Root Two Child",
			parent_chat_id: rootTwo.id,
			root_chat_id: rootTwo.id,
		});

		render(
			<Wrapper>
				<AgentsSidebar
					{...defaultProps}
					chats={[rootOne, rootOneChild, rootTwo, rootTwoChild]}
				/>
			</Wrapper>,
		);

		expect(
			screen.queryByTestId(`agents-tree-node-${rootOneChild.id}`),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId(`agents-tree-node-${rootTwoChild.id}`),
		).not.toBeInTheDocument();
		expect(
			screen.getByTestId(`agents-tree-node-${rootTwo.id}`),
		).toBeInTheDocument();

		fireEvent.click(screen.getByTestId(`agents-tree-toggle-${rootOne.id}`));

		expect(
			screen.getByTestId(`agents-tree-node-${rootOneChild.id}`),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId(`agents-tree-node-${rootTwoChild.id}`),
		).not.toBeInTheDocument();
		expect(
			screen.getByTestId(`agents-tree-node-${rootTwo.id}`),
		).toBeInTheDocument();
		expect(
			within(screen.getByTestId(`agents-tree-node-${rootTwo.id}`)).getByText(
				rootTwo.title,
			),
		).toBeInTheDocument();
	});
});
