import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import { MockChat, MockChatDiffStatus } from "#/testHelpers/chatEntities";
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

const mockDashboardValue = {
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
					<DashboardContext.Provider value={mockDashboardValue}>
						<MemoryRouter initialEntries={["/agents/chat-1"]}>
							{children}
						</MemoryRouter>
					</DashboardContext.Provider>
				</TooltipProvider>
			</ThemeOverride>
		</QueryClientProvider>
	);
};

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
	it("links every tracked PR as a menu anchor when the chat tracks several", async () => {
		const user = userEvent.setup();

		// Both PRs share a title, so the numbers must name them apart.
		const primary = { ...MockChatDiffStatus };
		const secondary = {
			...MockChatDiffStatus,
			url: "https://github.com/coder/coder/pull/456",
			pr_number: 456,
			git_branch: "feat/two",
		};
		renderTopBar({
			...MockChat,
			diff_statuses: [primary, secondary],
		});

		await user.click(screen.getByRole("button", { name: /2 PRs/ }));
		const menu = await screen.findByRole("menu");

		// Real anchors: middle-click and copy-link work, and the
		// destination is announced.
		expect(
			within(menu).getByRole("menuitem", { name: /PR #123/ }),
		).toHaveAttribute("href", "https://github.com/coder/coder/pull/123");
		expect(
			within(menu).getByRole("menuitem", { name: /PR #456/ }),
		).toHaveAttribute("href", "https://github.com/coder/coder/pull/456");
	});

	it("names the repository when two origins carry the same PR", async () => {
		const user = userEvent.setup();

		// The PR number repeats across repositories, so only the
		// repository names keep the entries apart.
		const forked = {
			...MockChatDiffStatus,
			remote_origin: "https://github.com/coder/other-project.git",
			git_branch: "feat/two",
			url: "https://github.com/coder/other-project/pull/123",
		};
		renderTopBar({
			...MockChat,
			diff_statuses: [MockChatDiffStatus, forked],
		});

		await user.click(screen.getByRole("button", { name: /2 PRs/ }));
		const menu = await screen.findByRole("menu");

		expect(
			within(menu).getByRole("menuitem", { name: /coder\/coder · PR #123/ }),
		).toHaveAttribute("href", "https://github.com/coder/coder/pull/123");
		expect(
			within(menu).getByRole("menuitem", {
				name: /coder\/other-project · PR #123/,
			}),
		).toHaveAttribute(
			"href",
			"https://github.com/coder/other-project/pull/123",
		);
	});
});
