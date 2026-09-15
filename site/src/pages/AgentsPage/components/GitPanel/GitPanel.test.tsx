import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatDiffContents, ChatDiffStatus } from "#/api/typesGenerated";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes, { DEFAULT_THEME } from "#/theme";
import { GitPanel } from "./GitPanel";

const prStatus = (overrides: Partial<ChatDiffStatus> = {}): ChatDiffStatus => ({
	chat_id: "test-chat",
	pull_request_title: "",
	pull_request_draft: false,
	changes_requested: false,
	additions: 0,
	deletions: 0,
	changed_files: 0,
	pull_request_state: "open",
	remote_origin: "https://github.com/coder/coder",
	...overrides,
});

const diffContents = (chatId: string): ChatDiffContents => ({
	chat_id: chatId,
});

const Wrapper: FC<PropsWithChildren> = ({ children }) => {
	const queryClient = createTestQueryClient();
	return (
		<QueryClientProvider client={queryClient}>
			<ThemeOverride theme={themes[DEFAULT_THEME]}>{children}</ThemeOverride>
		</QueryClientProvider>
	);
};

const renderPanel = (props: Partial<React.ComponentProps<typeof GitPanel>>) => {
	return render(
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
};

describe("GitPanel per-ref views", () => {
	it("fetches the selected ref's diff, not the primary's", async () => {
		const user = userEvent.setup();
		const getDiff = vi
			.spyOn(API.experimental, "getChatDiffContents")
			.mockResolvedValue(diffContents("test-chat"));

		renderPanel({
			remoteDiffStats: [
				prStatus({
					pull_request_title: "feat: first change",
					git_branch: "feat/first",
					pr_number: 23020,
					url: "https://github.com/coder/coder/pull/23020",
				}),
				prStatus({
					pull_request_title: "fix: second change",
					git_branch: "fix/second",
					pr_number: 23021,
					url: "https://github.com/coder/coder/pull/23021",
					pull_request_state: "merged",
				}),
			],
		});

		// The default view targets the primary ref.
		await waitFor(() =>
			expect(getDiff).toHaveBeenCalledWith(
				"test-chat",
				expect.objectContaining({
					remote_origin: "https://github.com/coder/coder",
					git_branch: "feat/first",
				}),
			),
		);

		await user.click(screen.getByTestId("git-panel-view-switcher"));
		const menu = await screen.findByRole("menu");
		await user.click(within(menu).getByText("PR #23021"));

		// After the switch, the fetch must target the second ref.
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
			.mockResolvedValue(diffContents("test-chat"));

		renderPanel({
			remoteDiffStats: [
				prStatus({
					git_branch: "feature/no-pr-yet",
					url: undefined,
					pull_request_state: undefined,
				}),
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
});
