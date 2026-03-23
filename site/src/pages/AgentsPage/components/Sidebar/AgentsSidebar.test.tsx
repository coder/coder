import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
	MockUserOwner,
} from "testHelpers/entities";
import { act, render } from "@testing-library/react";
import type * as TypesGen from "api/typesGenerated";
import type { Chat } from "api/typesGenerated";
import { ThemeOverride } from "contexts/ThemeProvider";
import { DashboardContext } from "modules/dashboard/DashboardProvider";
import type { FC, PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { MemoryRouter } from "react-router";
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
