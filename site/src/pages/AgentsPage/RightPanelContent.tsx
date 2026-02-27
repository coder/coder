import { parsePatchFiles } from "@pierre/diffs";
import { FileDiff } from "@pierre/diffs/react";
import { chatDiffStatus } from "api/queries/chats";
import type { WorkspaceAgentRepoChanges } from "api/typesGenerated";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import { GitPullRequestIcon, MonitorIcon } from "lucide-react";
import { type FC, useMemo, useState } from "react";
import { useQuery } from "react-query";
import { cn } from "utils/cn";
import { FilesChangedPanel } from "./FilesChangedPanel";

interface RightPanelContentProps {
	chatId: string;
	gitChangesRepos: readonly WorkspaceAgentRepoChanges[] | undefined;
}

type Tab = { id: string; label: string; type: "pr" | "workspace" };

function extractPrNumber(url: string): string | undefined {
	const match = url.match(/\/pull\/(\d+)/);
	return match ? match[1] : undefined;
}

const diffOptions = {
	diffStyle: "unified" as const,
	diffIndicators: "bars" as const,
	overflow: "scroll" as const,
	themeType: "dark" as const,
	enableLineSelection: true,
	enableHoverUtility: true,
	theme: "github-dark-high-contrast" as const,
	unsafeCSS:
		"pre, [data-line], [data-diffs-header] { background-color: transparent !important; } [data-diffs-header] { border-left: 1px solid var(--border); }",
};

const diffStyle = {
	"--diffs-font-size": "11px",
	"--diffs-line-height": "1.5",
};

const WorkspaceDiffPanel: FC<{ diff: string; label: string }> = ({
	diff,
	label,
}) => {
	const parsedFiles = useMemo(() => {
		if (!diff) return [];
		try {
			return parsePatchFiles(diff).flatMap((p) => p.files);
		} catch {
			return [];
		}
	}, [diff]);

	if (parsedFiles.length === 0) {
		return (
			<div className="flex h-full items-center justify-center border-0 border-l border-solid p-6 text-center text-xs text-content-secondary">
				No workspace changes to display.
			</div>
		);
	}

	return (
		<div className="flex h-full flex-col border-0 border-l border-solid">
			<div className="flex items-center justify-between gap-3 border-0 border-b border-solid px-4 py-3">
				<div className="flex items-center gap-2">
					<MonitorIcon className="h-4 w-4 shrink-0 text-content-secondary" />
					<span className="text-sm font-medium text-content-primary">
						Working changes
					</span>
				</div>
				<span className="truncate text-xs text-content-secondary">{label}</span>
			</div>
			<ScrollArea className="min-w-0 flex-1" scrollBarClassName="w-1.5">
				<div className="min-w-0 text-xs">
					{parsedFiles.map((fileDiff) => (
						<FileDiff
							key={fileDiff.name}
							fileDiff={fileDiff}
							options={diffOptions}
							style={diffStyle}
						/>
					))}
				</div>
			</ScrollArea>
		</div>
	);
};

export const RightPanelContent: FC<RightPanelContentProps> = ({
	chatId,
	gitChangesRepos,
}) => {
	const [activeTab, setActiveTab] = useState("pr");
	const diffStatusQuery = useQuery(chatDiffStatus(chatId));
	const repos = gitChangesRepos;

	const prNumber = useMemo(() => {
		const url = diffStatusQuery.data?.url;
		return url ? extractPrNumber(url) : undefined;
	}, [diffStatusQuery.data?.url]);

	const workspaceTabs: Tab[] = useMemo(() => {
		if (!repos || repos.length === 0) return [];
		return repos.map((repo) => {
			// Derive a short label from the repo root (last directory component)
			// or the remote origin.
			const parts = repo.repo_root.split("/");
			const dirName = parts[parts.length - 1] || repo.repo_root;
			return {
				id: repo.repo_root,
				label: dirName,
				type: "workspace" as const,
			};
		});
	}, [repos]);

	const prTab: Tab = {
		id: "pr",
		label: prNumber ? `#${prNumber}` : "PR",
		type: "pr",
	};
	const tabs = [prTab, ...workspaceTabs];

	// Find the active repo's diff data.
	const activeRepo = repos?.find((r) => r.repo_root === activeTab);

	return (
		<div className="flex h-full flex-col overflow-hidden">
			{/* Tab bar */}
			<div className="flex gap-1 border-0 border-b border-l border-solid px-2 py-1.5">
				{tabs.map((tab) => (
					<button
						key={tab.id}
						type="button"
						onClick={() => setActiveTab(tab.id)}
						className={cn(
							"flex items-center gap-1.5 rounded-md border-none px-2.5 py-1 text-xs transition-colors",
							activeTab === tab.id
								? "bg-surface-tertiary text-content-primary"
								: "bg-transparent text-content-secondary hover:bg-surface-secondary hover:text-content-primary",
						)}
					>
						{tab.type === "pr" ? (
							<GitPullRequestIcon className="h-3 w-3" />
						) : (
							<MonitorIcon className="h-3 w-3" />
						)}
						{tab.label}
					</button>
				))}
			</div>

			{/* Panel content */}
			<div className="min-h-0 flex-1">
				{activeTab === "pr" ? (
					<FilesChangedPanel chatId={chatId} />
				) : activeRepo ? (
					<WorkspaceDiffPanel
						diff={activeRepo.unified_diff}
						label={tabs.find((t) => t.id === activeTab)?.label ?? activeTab}
					/>
				) : (
					<div className="flex h-full items-center justify-center border-0 border-l border-solid p-6 text-center text-xs text-content-secondary">
						No workspace changes to display.
					</div>
				)}
			</div>
		</div>
	);
};
