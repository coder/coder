import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
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
import { ChatTopBar } from "./ChatTopBar";

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserOwner,
		permissions: {},
		signOut: vi.fn(),
	}),
}));

const dashboardValue = {
	entitlements: MockEntitlements,
	experiments: [],
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
					<DashboardContext.Provider value={dashboardValue}>
						<MemoryRouter initialEntries={["/agents/chat-1"]}>
							{children}
						</MemoryRouter>
					</DashboardContext.Provider>
				</TooltipProvider>
			</ThemeOverride>
		</QueryClientProvider>
	);
};

const prStatus = (
	overrides: Partial<NonNullable<Chat["diff_statuses"]>[number]> = {},
) => ({
	chat_id: "chat-1",
	url: "https://github.com/coder/coder/pull/123",
	pull_request_title: "fix: resolve race condition",
	pull_request_draft: false,
	changes_requested: false,
	additions: 1,
	deletions: 0,
	changed_files: 1,
	...overrides,
});

const renderTopBar = (chat: Chat) => {
	render(
		<Wrapper>
			<ChatTopBar
				chat={chat}
				panel={{ showSidebarPanel: false, onToggleSidebar: () => {} }}
			/>
		</Wrapper>,
	);
};

describe("ChatTopBar PR chip", () => {
	it("renders one PR as a direct link", async () => {
		const status = prStatus();
		renderTopBar({
			...MockChat,
			diff_statuses: [status],
		});

		const link = screen.getByRole("link", {
			name: /fix: resolve race condition/,
		});
		expect(link).toHaveAttribute(
			"href",
			"https://github.com/coder/coder/pull/123",
		);
		expect(
			screen.queryByLabelText("View pull requests"),
		).not.toBeInTheDocument();
	});

	it("lists every PR in a menu when the chat tracks several", async () => {
		const primary = prStatus();
		const secondary = prStatus({
			url: "https://github.com/coder/coder/pull/456",
			pull_request_title: "feat: add notification system",
			pull_request_draft: true,
			git_branch: "feat/two",
		});
		renderTopBar({
			...MockChat,
			diff_statuses: [primary, secondary],
		});

		await userEvent.click(screen.getByLabelText("View pull requests"));

		await waitFor(() => {
			const items = screen.getAllByRole("menuitem");
			expect(items).toHaveLength(2);
		});
		expect(
			screen.getByRole("menuitem", {
				name: /fix: resolve race condition/,
			}),
		).toBeInTheDocument();
		expect(
			screen.getByRole("menuitem", {
				name: /feat: add notification system/,
			}),
		).toBeInTheDocument();
	});

	it("ignores tracked refs without a pull request link", async () => {
		const primary = prStatus();
		const noPR = prStatus({
			url: undefined,
			pull_request_title: "",
			git_branch: "feat/no-pr",
		});
		renderTopBar({
			...MockChat,
			diff_statuses: [primary, noPR],
		});

		const link = screen.getByRole("link", {
			name: /fix: resolve race condition/,
		});
		expect(link).toBeInTheDocument();
		expect(
			screen.queryByLabelText("View pull requests"),
		).not.toBeInTheDocument();
	});
});
