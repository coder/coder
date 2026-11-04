import type {
	ChatDiffStatus,
	WorkspaceAgentRepoChanges,
} from "#/api/typesGenerated";

/** A Git panel view: a tracked ref's diff, or a repo's local changes. */
export type GitView =
	| { type: "remote"; refId: string }
	| { type: "local"; repoRoot: string };

/** A request from outside the Git panel to show `view`, once per `id`. */
export type GitViewRequest = { id: number; view: GitView };

/**
 * The view id for a tracked ref. Rows without origin and branch share the
 * empty-key id.
 */
export const viewIdFor = (status: ChatDiffStatus): string =>
	`remote:${status.remote_origin ?? ""}:${status.git_branch ?? ""}`;

/** Added and deleted line counts of one diff. */
export type DiffStats = {
	additions: number;
	deletions: number;
};

/** Line counts of a repo's uncommitted diff, zero when it has none. */
export const countRepoChanges = (
	repo: WorkspaceAgentRepoChanges,
): DiffStats => {
	let additions = 0;
	let deletions = 0;
	for (const line of (repo.unified_diff ?? "").split("\n")) {
		if (line.startsWith("+") && !line.startsWith("+++")) {
			additions++;
		} else if (line.startsWith("-") && !line.startsWith("---")) {
			deletions++;
		}
	}
	return { additions, deletions };
};

/** Per-repo diff stats. Repos with no added or deleted lines are left out. */
export const computeRepoStats = (
	repositories: ReadonlyMap<string, WorkspaceAgentRepoChanges>,
): Map<string, DiffStats> => {
	const stats = new Map<string, DiffStats>();
	for (const [root, repo] of repositories.entries()) {
		const repoStat = countRepoChanges(repo);
		if (repoStat.additions > 0 || repoStat.deletions > 0) {
			stats.set(root, repoStat);
		}
	}
	return stats;
};

/** The last path segment of `repoRoot`, or `repoRoot` when it has none. */
export const repoLabel = (repoRoot: string): string => {
	const segments = repoRoot.split("/").filter(Boolean);
	return segments[segments.length - 1] ?? repoRoot;
};

/** A repo's branch, or its folder name for a detached or unnamed HEAD. */
export const repoBranchLabel = (
	repoRoot: string,
	repo: WorkspaceAgentRepoChanges,
): string => repo.branch?.trim() || repoLabel(repoRoot);
