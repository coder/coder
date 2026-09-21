import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
import type { ChatMessageInputRef } from "../AgentChatInput";
import { GitPanel } from "./GitPanel";

vi.mock("../DiffViewer/LocalDiffPanel", () => ({
	LocalDiffPanel: () => null,
}));

const mockDiffContents: ChatDiffContents = {
	chat_id: "test-chat",
};

const Wrapper: React.FC<React.PropsWithChildren> = ({ children }) => {
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
				repositories={new Map()}
				isExpanded={false}
				isGitStatusLoading={false}
				everDirty={new Set()}
				chatInputRef={{ current: null }}
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
					repositories={new Map()}
					isExpanded={false}
					isGitStatusLoading={false}
					everDirty={new Set()}
					chatInputRef={{ current: null }}
					{...next}
				/>
			</Wrapper>,
		);
	};
	return { view, rerenderPanel };
};

const repoRoot = "/workspace/special-repo";
const commitPrompt = `Commit and push the working changes in ${repoRoot}. If there are unstaged files, commit them too.`;

const repo: WorkspaceAgentRepoChanges = {
	repo_root: repoRoot,
	branch: "feat/example",
	unified_diff: `diff --git a/src/main.ts b/src/main.ts
index abc1234..def5678 100644
--- a/src/main.ts
+++ b/src/main.ts
@@ -1 +1,2 @@
 existing
+added
`,
};

function mockComposer(value = ""): ChatMessageInputRef {
	let current = value;
	return {
		getValue: () => current,
		insertText: vi.fn((text: string) => {
			current += text;
		}),
		setValue: vi.fn(),
		clear: vi.fn(),
		focus: vi.fn(),
		addFileReference: vi.fn(),
		getContentParts: vi.fn(() => []),
	};
}

function renderGitPanel(input: ChatMessageInputRef) {
	renderPanel({
		repositories: new Map([[repoRoot, repo]]),
		chatInputRef: { current: input },
	});
}

describe("GitPanel", () => {
	it("inserts a commit prompt for the active repo when Commit is clicked", async () => {
		const user = userEvent.setup();
		const input = mockComposer();
		renderGitPanel(input);

		await user.click(screen.getByRole("button", { name: "Commit" }));

		expect(input.insertText).toHaveBeenCalledWith(commitPrompt);
		expect(input.focus).toHaveBeenCalled();
	});

	it("does not insert the prompt twice for the same repo", async () => {
		const user = userEvent.setup();
		const input = mockComposer();
		renderGitPanel(input);

		const commit = screen.getByRole("button", { name: "Commit" });
		await user.click(commit);
		await user.click(commit);

		expect(input.insertText).toHaveBeenCalledTimes(1);
	});

	it("separates the prompt from existing composer text", async () => {
		const user = userEvent.setup();
		const input = mockComposer("please review");
		renderGitPanel(input);

		await user.click(screen.getByRole("button", { name: "Commit" }));

		expect(input.insertText).toHaveBeenCalledWith(`\n\n${commitPrompt}`);
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
