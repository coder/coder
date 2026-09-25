import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { WorkspaceAgentRepoChanges } from "#/api/typesGenerated";
import type { ChatMessageInputRef } from "../AgentChatInput";
import { GitPanel } from "./GitPanel";

vi.mock("../DiffViewer/DiffViewer", () => ({
	loadDiffStyle: () => "unified",
	saveDiffStyle: () => {},
}));

vi.mock("../DiffViewer/LocalDiffPanel", () => ({
	LocalDiffPanel: () => null,
}));

vi.mock("../DiffViewer/RemoteDiffPanel", () => ({
	RemoteDiffPanel: () => null,
}));

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
	render(
		<GitPanel
			repositories={new Map([[repoRoot, repo]])}
			onRefresh={() => true}
			chatInputRef={{ current: input }}
		/>,
	);
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
