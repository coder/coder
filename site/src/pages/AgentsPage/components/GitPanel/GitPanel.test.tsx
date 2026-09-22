import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type {
	ChatDiffContents,
	WorkspaceAgentRepoChanges,
} from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { MockChatDiffStatus } from "#/testHelpers/chatEntities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes, { DEFAULT_THEME } from "#/theme";
import { GitPanel } from "./GitPanel";

// The local diff renders a web component that jsdom cannot
// construct. The header tests only need the panel to mount.
vi.mock("../DiffViewer/LocalDiffPanel", () => ({
	LocalDiffPanel: () => <div data-testid="local-diff-panel" />,
}));

const mockDiffContents: ChatDiffContents = {
	chat_id: "test-chat",
};

const mockRepo: WorkspaceAgentRepoChanges = {
	repo_root: "/home/coder/coder",
	branch: "feat/add-logging",
	remote_origin: "https://github.com/coder/coder.git",
	unified_diff:
		"diff --git a/a.ts b/a.ts\n--- a/a.ts\n+++ b/a.ts\n@@ -1 +1 @@\n-old\n+new\n",
};

const Wrapper: FC<PropsWithChildren> = ({ children }) => {
	const queryClient = createTestQueryClient();
	return (
		<QueryClientProvider client={queryClient}>
			<ThemeOverride theme={themes[DEFAULT_THEME]}>
				<TooltipProvider>{children}</TooltipProvider>
			</ThemeOverride>
		</QueryClientProvider>
	);
};

const renderPanel = (props: Partial<React.ComponentProps<typeof GitPanel>>) => {
	const view = render(
		<Wrapper>
			<GitPanel
				chatId="test-chat"
				onRefresh={() => true}
				onCommit={() => {}}
				repositories={new Map()}
				{...props}
			/>
		</Wrapper>,
	);
	const rerenderPanel = (
		next: Partial<React.ComponentProps<typeof GitPanel>>,
	) => {
		view.rerender(
			<Wrapper>
				<GitPanel
					chatId="test-chat"
					onRefresh={() => true}
					onCommit={() => {}}
					repositories={new Map()}
					{...next}
				/>
			</Wrapper>,
		);
	};
	return { view, rerenderPanel };
};

describe("GitPanel header", () => {
	it("shows a lone PR as a label with a View PR link, not a menu", async () => {
		vi.spyOn(API.experimental, "getChatDiffContents").mockResolvedValue(
			mockDiffContents,
		);

		renderPanel({
			remoteDiffStats: [
				{
					...MockChatDiffStatus,
					pr_number: 4847,
					url: "https://github.com/coder/coder/pull/4847",
				},
			],
		});

		expect(screen.getByTestId("git-panel-view-switcher")).toHaveTextContent(
			"PR #4847",
		);
		expect(
			screen.queryByRole("button", { name: "Switch git view" }),
		).not.toBeInTheDocument();
		expect(screen.getByRole("link", { name: /View PR/ })).toHaveAttribute(
			"href",
			"https://github.com/coder/coder/pull/4847",
		);
		expect(
			screen.queryByRole("button", { name: /Commit/ }),
		).not.toBeInTheDocument();
	});

	it("hides View PR for a branch-only ref", async () => {
		vi.spyOn(API.experimental, "getChatDiffContents").mockResolvedValue(
			mockDiffContents,
		);

		renderPanel({
			remoteDiffStats: [
				{
					...MockChatDiffStatus,
					git_branch: "feat/branch-only",
					url: "https://github.com/coder/coder/tree/feat/branch-only",
					pr_number: undefined,
					pull_request_state: undefined,
					pull_request_title: "",
				},
			],
		});

		expect(screen.getByTestId("git-panel-view-switcher")).toHaveTextContent(
			"feat/branch-only",
		);
		expect(
			screen.queryByRole("link", { name: /View PR/ }),
		).not.toBeInTheDocument();
	});

	it("replaces View PR with a Commit button for a local-only repo", async () => {
		const user = userEvent.setup();
		const onCommit = vi.fn();

		renderPanel({
			onCommit,
			repositories: new Map([[mockRepo.repo_root, mockRepo]]),
		});

		expect(
			screen.queryByRole("button", { name: "Switch git view" }),
		).not.toBeInTheDocument();
		expect(
			screen.queryByRole("link", { name: /View PR/ }),
		).not.toBeInTheDocument();

		await user.click(screen.getByRole("button", { name: /Commit/ }));
		expect(onCommit).toHaveBeenCalledWith(mockRepo.repo_root);
	});

	it("swaps the header action when switching between a PR and a repo", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "getChatDiffContents").mockResolvedValue(
			mockDiffContents,
		);

		renderPanel({
			remoteDiffStats: [
				{
					...MockChatDiffStatus,
					pr_number: 4847,
					url: "https://github.com/coder/coder/pull/4847",
				},
			],
			repositories: new Map([[mockRepo.repo_root, mockRepo]]),
		});

		expect(screen.getByRole("link", { name: /View PR/ })).toBeInTheDocument();

		await user.click(screen.getByRole("button", { name: "Switch git view" }));
		const menu = await screen.findByRole("menu");
		await user.click(within(menu).getByText("Working"));

		expect(screen.getByRole("button", { name: /Commit/ })).toBeInTheDocument();
		expect(
			screen.queryByRole("link", { name: /View PR/ }),
		).not.toBeInTheDocument();
	});
});

describe("GitPanel per-ref views", () => {
	it("fetches the selected ref's diff, not the primary's", async () => {
		const user = userEvent.setup();
		const getDiff = vi
			.spyOn(API.experimental, "getChatDiffContents")
			.mockResolvedValue(mockDiffContents);

		renderPanel({
			remoteDiffStats: [
				{
					...MockChatDiffStatus,
					pull_request_title: "feat: first change",
					git_branch: "feat/first",
					pr_number: 23020,
					url: "https://github.com/coder/coder/pull/23020",
				},
				{
					...MockChatDiffStatus,
					pull_request_title: "fix: second change",
					git_branch: "fix/second",
					pr_number: 23021,
					url: "https://github.com/coder/coder/pull/23021",
					pull_request_state: "merged",
				},
			],
		});

		await waitFor(() =>
			expect(getDiff).toHaveBeenCalledWith(
				"test-chat",
				expect.objectContaining({
					remote_origin: "https://github.com/coder/coder",
					git_branch: "feat/first",
				}),
			),
		);

		await user.click(screen.getByRole("button", { name: "Switch git view" }));
		const menu = await screen.findByRole("menu");
		await user.click(within(menu).getByText("PR #23021"));

		await waitFor(() =>
			expect(getDiff).toHaveBeenLastCalledWith(
				"test-chat",
				expect.objectContaining({
					remote_origin: "https://github.com/coder/coder",
					git_branch: "fix/second",
				}),
			),
		);
	});

	it("fetches a branch-only ref's diff even without a PR URL", async () => {
		const getDiff = vi
			.spyOn(API.experimental, "getChatDiffContents")
			.mockResolvedValue(mockDiffContents);

		renderPanel({
			remoteDiffStats: [
				{
					...MockChatDiffStatus,
					git_branch: "feature/no-pr-yet",
					url: undefined,
					pr_number: undefined,
					pull_request_state: undefined,
					pull_request_title: "",
				},
			],
		});

		// The branch has no PR URL, but its ref selector must still
		// drive a diff fetch.
		await waitFor(() =>
			expect(getDiff).toHaveBeenCalledWith(
				"test-chat",
				expect.objectContaining({
					remote_origin: "https://github.com/coder/coder",
					git_branch: "feature/no-pr-yet",
				}),
			),
		);
	});

	it("adopts the first refs when they arrive after mount", async () => {
		const getDiff = vi
			.spyOn(API.experimental, "getChatDiffContents")
			.mockResolvedValue(mockDiffContents);

		const view = renderPanel({ remoteDiffStats: undefined });

		const firstRef = {
			...MockChatDiffStatus,
			pull_request_title: "feat: first change",
			git_branch: "feat/first",
			pr_number: 23020,
			url: "https://github.com/coder/coder/pull/23020",
		};
		const secondRef = {
			...MockChatDiffStatus,
			pull_request_title: "fix: second change",
			git_branch: "fix/second",
			pr_number: 23021,
			url: "https://github.com/coder/coder/pull/23021",
		};
		view.rerenderPanel({ remoteDiffStats: [firstRef, secondRef] });

		await waitFor(() =>
			expect(getDiff).toHaveBeenCalledWith(
				"test-chat",
				expect.objectContaining({
					remote_origin: "https://github.com/coder/coder",
					git_branch: "feat/first",
				}),
			),
		);
	});

	it("fetches the new primary's diff when a keyless primary is superseded", async () => {
		const getDiff = vi
			.spyOn(API.experimental, "getChatDiffContents")
			.mockResolvedValue(mockDiffContents);

		const legacyKeyless = {
			...MockChatDiffStatus,
			remote_origin: "",
			git_branch: "",
			pull_request_title: "fix: legacy change",
			pr_number: 23020,
			url: "https://github.com/coder/coder/pull/23020",
		};
		const keyedRef = {
			...MockChatDiffStatus,
			pull_request_title: "fix: keyed change",
			git_branch: "fix/keyed",
			pr_number: 23021,
			url: "https://github.com/coder/coder/pull/23021",
		};

		// A chat upgraded from the unkeyed schema starts with the
		// legacy row as its only ref.
		const { rerenderPanel } = renderPanel({ remoteDiffStats: [legacyKeyless] });

		// The agent later reports a keyed ref, which becomes the
		// primary and demotes the legacy row behind it.
		rerenderPanel({ remoteDiffStats: [keyedRef, legacyKeyless] });

		await waitFor(() =>
			expect(getDiff).toHaveBeenLastCalledWith(
				"test-chat",
				expect.objectContaining({
					remote_origin: "https://github.com/coder/coder",
					git_branch: "fix/keyed",
				}),
			),
		);
	});
});
