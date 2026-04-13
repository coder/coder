import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, spyOn } from "storybook/test";
import { API } from "#/api/api";
import type { ChatDiffContents, ChatDiffStatus } from "#/api/typesGenerated";
import { GitPanel } from "./GitPanel";

interface LargeDiffFixture {
	diff: string;
	changedFiles: number;
	additions: number;
	deletions: number;
}

const repeatToken = (value: string, count: number): string =>
	Array.from({ length: count }, (_, index) => `${value}_${index}`).join(" ");

const makeWrappedLine = (fileIndex: number, lineIndex: number): string => {
	return [
		`wrapped_${fileIndex}_${lineIndex}`,
		repeatToken(`segment_${fileIndex}_${lineIndex}`, 12),
		repeatToken("performance_probe", 6),
	].join(" ");
};

const buildLargePullRequestDiff = (): LargeDiffFixture => {
	const changedFiles = 32;
	const changedLinePairsPerFile = 36;
	const diffParts: string[] = [];
	let additions = 0;
	let deletions = 0;

	for (let fileIndex = 0; fileIndex < changedFiles; fileIndex++) {
		const filePath = `packages/perf-suite/src/generated/perfFile${String(fileIndex + 1).padStart(2, "0")}.ts`;

		diffParts.push(`diff --git a/${filePath} b/${filePath}`);
		diffParts.push(
			`index ${String(fileIndex).padStart(7, "0")}..${String(fileIndex + 1).padStart(7, "0")} 100644`,
		);
		diffParts.push(`--- a/${filePath}`);
		diffParts.push(`+++ b/${filePath}`);
		diffParts.push(
			`@@ -1,${changedLinePairsPerFile} +1,${changedLinePairsPerFile} @@`,
		);

		for (let lineIndex = 0; lineIndex < changedLinePairsPerFile; lineIndex++) {
			diffParts.push(
				`-export const oldValue_${fileIndex}_${lineIndex} = "removed ${fileIndex}:${lineIndex}";`,
			);
			diffParts.push(
				`+export const newValue_${fileIndex}_${lineIndex} = "${makeWrappedLine(fileIndex, lineIndex)}";`,
			);
			additions++;
			deletions++;
		}
	}

	return {
		diff: diffParts.join("\n"),
		changedFiles,
		additions,
		deletions,
	};
};

const largePullRequest = buildLargePullRequestDiff();

const defaultDiffContents: ChatDiffContents = {
	chat_id: "gitpanel-large-pr-perf",
};

const largePrStatus: ChatDiffStatus = {
	chat_id: "gitpanel-large-pr-perf",
	url: "https://github.com/coder/coder/pull/99999",
	pull_request_title:
		"perf: large wrapped PR fixture for GitPanel investigation",
	pull_request_state: "open",
	pull_request_draft: false,
	base_branch: "main",
	head_branch: "perf/large-gitpanel-fixture",
	changes_requested: false,
	additions: largePullRequest.additions,
	deletions: largePullRequest.deletions,
	changed_files: largePullRequest.changedFiles,
};

const meta: Meta<typeof GitPanel> = {
	title: "pages/AgentsPage/GitPanelPerf",
	component: GitPanel,
	args: {
		prTab: { prNumber: 99999, chatId: "gitpanel-large-pr-perf" },
		remoteDiffStats: largePrStatus,
		repositories: new Map(),
		onRefresh: fn().mockReturnValue(true),
		onCommit: fn(),
		isExpanded: true,
	},
	decorators: [
		(Story) => (
			<div style={{ height: 900, width: 1280 }}>
				<Story />
			</div>
		),
	],
	beforeEach: () => {
		spyOn(API.experimental, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: largePullRequest.diff,
		});
	},
};

export default meta;

type Story = StoryObj<typeof GitPanel>;

/**
 * 32 files × 36 deletion/addition pairs = 2,304 changed lines.
 * Added lines are intentionally long to force wrapping and stress
 * virtualized height reconciliation inside the GitPanel diff viewer.
 */
export const LargeWrappedPullRequest: Story = {};

/**
 * Same fixture, but below the file-tree threshold so the diff list renders
 * without the sidebar. Useful for isolating file-tree / active-file work.
 */
export const LargeWrappedPullRequestNarrow: Story = {
	args: {
		isExpanded: false,
	},
	decorators: [
		(Story) => (
			<div style={{ height: 900, width: 900 }}>
				<Story />
			</div>
		),
	],
};
