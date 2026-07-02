import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import type { Chat } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
	MockUserOwner,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes, { DEFAULT_THEME } from "#/theme";
import type { AgentSidebarFilters } from "../../utils/agentSidebarFilters";
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

const Wrapper: FC<PropsWithChildren> = ({ children }) => {
	const queryClient = createTestQueryClient();
	return (
		<QueryClientProvider client={queryClient}>
			<ThemeOverride theme={themes[DEFAULT_THEME]}>
				<TooltipProvider>
					<MemoryRouter initialEntries={["/agents"]}>
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
	chatStatuses: ["unread", "read"],
	sources: ["created_by_me"],
};

const defaultProps: React.ComponentProps<typeof ChatsSidebar> = {
	chats: [buildChat({ id: "chat-1", title: "Chat One" })],
	chatErrorReasons: {},
	modelOptions: [],
	modelConfigs: [],
	onArchiveAgent: vi.fn(),
	onUnarchiveAgent: vi.fn(),
	onArchiveAndDeleteWorkspace: vi.fn(),
	onPinAgent: vi.fn(),
	onUnpinAgent: vi.fn(),
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

describe("ChatsSidebar filters", () => {
	it("calls the sidebar filter change callback after Apply is clicked", async () => {
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
		await user.click(screen.getByRole("radio", { name: "Archived" }));

		expect(onSidebarFiltersChange).not.toHaveBeenCalled();

		await user.click(screen.getByRole("button", { name: "Apply" }));

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
			chatStatuses: ["unread"],
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
			chatStatuses: ["unread", "read"],
			sources: ["created_by_me"],
		});
	});

	it("applies source filters", async () => {
		const user = userEvent.setup();
		const onSidebarFiltersChange = vi.fn();

		const { rerender } = render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					sidebarFilters={defaultSidebarFilters}
					onSidebarFiltersChange={onSidebarFiltersChange}
				/>
			</Wrapper>,
		);

		await user.click(screen.getByRole("button", { name: "Filter agents" }));
		await user.click(screen.getByRole("checkbox", { name: "Shared with me" }));
		await user.click(screen.getByRole("button", { name: "Apply" }));

		expect(onSidebarFiltersChange).toHaveBeenLastCalledWith({
			...defaultSidebarFilters,
			sources: ["created_by_me", "shared_with_me"],
		});

		rerender(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					sidebarFilters={{
						...defaultSidebarFilters,
						sources: ["created_by_me", "shared_with_me"],
					}}
					onSidebarFiltersChange={onSidebarFiltersChange}
				/>
			</Wrapper>,
		);

		await user.click(screen.getByRole("button", { name: "Filter agents" }));
		await user.click(screen.getByRole("checkbox", { name: "Created by me" }));
		await user.click(screen.getByRole("button", { name: "Apply" }));

		expect(onSidebarFiltersChange).toHaveBeenLastCalledWith({
			...defaultSidebarFilters,
			sources: ["shared_with_me"],
		});
	});

	it("groups unpinned chats by chat status", () => {
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
						buildChat({
							id: "read-chat",
							title: "Read chat",
						}),
					]}
					sidebarFilters={{
						...defaultSidebarFilters,
						groupBy: "chat_status",
					}}
				/>
			</Wrapper>,
		);

		const unreadSection = screen.getByTestId("agents-section-toggle-Unread");
		const readSection = screen.getByTestId("agents-section-toggle-Read");
		const unreadNode = screen.getByTestId("agents-tree-node-unread-chat");
		const readNode = screen.getByTestId("agents-tree-node-read-chat");

		expect(
			screen.queryByTestId("agents-section-toggle-Today"),
		).not.toBeInTheDocument();
		expect(
			unreadSection.compareDocumentPosition(unreadNode) &
				Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
		expect(
			unreadNode.compareDocumentPosition(readSection) &
				Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
		expect(
			readSection.compareDocumentPosition(readNode) &
				Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
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

describe("ChatsSidebar model display names", () => {
	it("uses the chat model config ID to pick the correct duplicate model label", () => {
		const modelOptions = [
			{
				id: "config-fast",
				provider: "openai",
				model: "gpt-4o",
				displayName: "GPT-4o (Fast)",
			},
			{
				id: "config-quality",
				provider: "openai",
				model: "gpt-4o",
				displayName: "GPT-4o (Quality)",
			},
		];
		const modelConfigs: TypesGen.ChatModelConfig[] = [
			{
				id: "config-fast",
				ai_provider_id: "prov-openai",
				model: "gpt-4o",
				display_name: "GPT-4o (Fast)",
				enabled: true,
				is_default: false,
				context_limit: 128_000,
				compression_threshold: 70,
				created_at: oneWeekAgo,
				updated_at: oneWeekAgo,
			},
			{
				id: "config-quality",
				ai_provider_id: "prov-openai",
				model: "gpt-4o",
				display_name: "GPT-4o (Quality)",
				enabled: true,
				is_default: false,
				context_limit: 128_000,
				compression_threshold: 70,
				created_at: oneWeekAgo,
				updated_at: oneWeekAgo,
			},
		];

		const { getByText, queryByText } = render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					chats={[
						buildChat({
							id: "chat-quality",
							title: "Quality chat",
							last_model_config_id: "config-quality",
						}),
					]}
					modelOptions={modelOptions}
					modelConfigs={modelConfigs}
				/>
			</Wrapper>,
		);

		expect(getByText("GPT-4o (Quality)")).toBeInTheDocument();
		expect(queryByText("GPT-4o (Fast)")).not.toBeInTheDocument();
	});

	it("shows Default model when last_model_config_id is a nil UUID", () => {
		const { getByText } = render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					chats={[
						buildChat({
							id: "nil-uuid-chat",
							title: "Chat from pubsub",
							last_model_config_id: "00000000-0000-0000-0000-000000000000",
						}),
					]}
					modelOptions={[
						{
							id: "config-real",
							provider: "openai",
							model: "gpt-4o",
							displayName: "GPT-4o",
						},
					]}
				/>
			</Wrapper>,
		);

		// A nil UUID means LastModelConfigID was left at its zero value,
		// so the sidebar cannot resolve the model and falls back.
		expect(getByText("Default model")).toBeInTheDocument();
	});

	it("shows model name when last_model_config_id matches a config", () => {
		const { getByText, queryByText } = render(
			<Wrapper>
				<ChatsSidebar
					{...defaultProps}
					chats={[
						buildChat({
							id: "matched-chat",
							title: "Chat with valid model",
							last_model_config_id: "config-real",
						}),
					]}
					modelOptions={[
						{
							id: "config-real",
							provider: "openai",
							model: "gpt-4o",
							displayName: "GPT-4o",
						},
					]}
				/>
			</Wrapper>,
		);

		// Regression guard: a valid last_model_config_id must resolve
		// to the actual model display name, not "Default model".
		expect(getByText("GPT-4o")).toBeInTheDocument();
		expect(queryByText("Default model")).not.toBeInTheDocument();
	});
});

describe("ChatsSidebar subtitles", () => {
	const modelOptions = [
		{
			id: "model-1",
			provider: "openai",
			model: "gpt-4o",
			displayName: "GPT-4o",
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
					modelOptions={modelOptions}
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
					modelOptions={modelOptions}
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
					modelOptions={modelOptions}
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
					modelOptions={modelOptions}
				/>
			</Wrapper>,
		);

		expect(screen.getByText("GPT-4o")).toBeInTheDocument();
	});
});
