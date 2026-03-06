import { parsePatchFiles } from "@pierre/diffs";
import type { WorkspaceAgentRepoChanges } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	CheckIcon,
	FolderIcon,
	GitBranchIcon,
	RefreshCwIcon,
} from "lucide-react";
import {
	type FC,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";
import { DiffViewer } from "./DiffViewer";

interface RepoChangesPanelProps {
	repo: WorkspaceAgentRepoChanges;
	onRefresh: () => void;
	onCommit: () => void;
	isExpanded?: boolean;
}

function splitRepoPath(repoRoot: string): { parent: string; name: string } {
	const lastSlash = repoRoot.lastIndexOf("/");
	if (lastSlash === -1) {
		return { parent: "", name: repoRoot };
	}
	return {
		parent: repoRoot.slice(0, lastSlash + 1),
		name: repoRoot.slice(lastSlash + 1),
	};
}

export const RepoChangesPanel: FC<RepoChangesPanelProps> = ({
	repo,
	onRefresh,
	onCommit,
	isExpanded,
}) => {
	const [spinning, setSpinning] = useState(false);
	const spinTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
	useEffect(() => () => clearTimeout(spinTimerRef.current), []);
	const handleRefresh = useCallback(() => {
		onRefresh();
		setSpinning(true);
		clearTimeout(spinTimerRef.current);
		spinTimerRef.current = setTimeout(() => setSpinning(false), 1000);
	}, [onRefresh]);

	const parsedFiles = useMemo(() => {
		const diff = repo.unified_diff;
		if (!diff) {
			return [];
		}
		try {
			const patches = parsePatchFiles(diff);
			return patches.flatMap((p) => p.files);
		} catch {
			return [];
		}
	}, [repo.unified_diff]);

	const { parent: repoParent, name: repoName } = splitRepoPath(repo.repo_root);
	const hasChanges = parsedFiles.length > 0;

	return (
		<DiffViewer
			headerLeft={
				<div className="flex w-full min-w-0 items-center gap-1.5">
					<FolderIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
					<span className="shrink-0 text-xs font-medium text-content-primary">
						{repoName}
					</span>
					<span className="truncate text-xs text-content-secondary">
						{repoParent}
					</span>
					{repo.branch?.trim() && (
						<div className="hidden items-center gap-1 rounded-md border border-solid border-border-default px-1.5 py-0.5 text-xs text-content-secondary sm:flex">
							<GitBranchIcon className="h-3 w-3 shrink-0" />
							<span className="truncate">{repo.branch}</span>
						</div>
					)}
					<div className="ml-auto flex shrink-0 items-center gap-1">
						<Button
							size="sm"
							onClick={onCommit}
							disabled={!hasChanges}
							className="h-6 gap-1.5 border border-transparent bg-surface-invert-primary px-2 text-xs text-content-invert hover:bg-surface-invert-secondary active:opacity-80"
						>
							<CheckIcon className="h-3 w-3" />
							Commit
						</Button>
						<Button
							variant="subtle"
							size="icon"
							onClick={handleRefresh}
							aria-label="Refresh"
							className="h-6 w-6 text-content-secondary hover:text-content-primary"
						>
							<RefreshCwIcon
								className={cn(
									"h-3.5 w-3.5",
									spinning && "motion-safe:animate-spin-once",
								)}
							/>
						</Button>
					</div>
				</div>
			}
			parsedFiles={parsedFiles}
			isExpanded={isExpanded}
			emptyMessage="No file changes."
		/>
	);
};
