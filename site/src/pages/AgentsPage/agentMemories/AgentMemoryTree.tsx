import {
	ChevronDownIcon,
	ChevronRightIcon,
	FileTextIcon,
	FolderIcon,
	FolderOpenIcon,
} from "lucide-react";
import { type FC, useEffect } from "react";
import { useInfiniteQuery } from "react-query";
import { agentMemoryChildren } from "#/api/queries/agentMemories";
import type { AgentMemoryEntry } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";

type AgentMemoryTreeProps = {
	selectedPath?: string;
	expanded: ReadonlySet<string>;
	onToggle: (path: string) => void;
	onSelect: (entry: AgentMemoryEntry) => void;
};

const nameFromPath = (path: string) => path.slice(path.lastIndexOf("/") + 1);

const treeRowClasses =
	"flex h-8 w-full items-center gap-1.5 rounded-md border-0 bg-transparent px-2 text-left text-sm " +
	"focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link focus-visible:ring-inset";

const Directory: FC<AgentMemoryTreeProps & { directory: string }> = ({
	directory,
	selectedPath,
	expanded,
	onToggle,
	onSelect,
}) => {
	const isExpanded = directory === "/" || expanded.has(directory);
	const query = useInfiniteQuery({
		queryKey: agentMemoryChildren(directory, 0).queryKey.slice(0, -1),
		queryFn: ({ pageParam }) =>
			agentMemoryChildren(directory, pageParam).queryFn(),
		initialPageParam: 0,
		getNextPageParam: (lastPage) => lastPage.next_offset,
		enabled: isExpanded,
	});
	const entries = query.data?.pages.flatMap((page) => page.entries) ?? [];
	const selectedChildPath = selectedPath?.startsWith(
		directory === "/" ? "/" : `${directory}/`,
	)
		? `${directory === "/" ? "" : directory}/${
				selectedPath
					.slice(directory === "/" ? 1 : directory.length + 1)
					.split("/")[0]
			}`
		: undefined;

	useEffect(() => {
		if (
			selectedChildPath &&
			!entries.some((entry) => entry.path === selectedChildPath) &&
			query.hasNextPage &&
			!query.isFetchingNextPage
		) {
			void query.fetchNextPage();
		}
	}, [entries, query, selectedChildPath]);

	if (!isExpanded) {
		return null;
	}
	if (query.isLoading) {
		return (
			<div className="flex h-8 items-center gap-2 px-2 text-xs text-content-secondary">
				<Spinner loading size="sm" /> Loading
			</div>
		);
	}
	if (query.error) {
		return (
			<div className="flex items-center gap-2 px-2 py-1 text-xs text-content-destructive">
				Could not load folder.
				<Button
					size="sm"
					variant="outline"
					onClick={() => void query.refetch()}
				>
					Retry
				</Button>
			</div>
		);
	}

	const children = (
		<>
			{entries.map((entry) => {
				if (entry.kind === "directory") {
					const childExpanded = expanded.has(entry.path);
					return (
						<div key={entry.path}>
							<button
								type="button"
								role="treeitem"
								aria-expanded={childExpanded}
								className={`${treeRowClasses} text-content-secondary hover:bg-surface-secondary hover:text-content-primary`}
								onClick={() => onToggle(entry.path)}
							>
								{childExpanded ? (
									<ChevronDownIcon className="size-4 shrink-0" />
								) : (
									<ChevronRightIcon className="size-4 shrink-0" />
								)}
								{childExpanded ? (
									<FolderOpenIcon className="size-4 shrink-0" />
								) : (
									<FolderIcon className="size-4 shrink-0" />
								)}
								<span className="truncate">{nameFromPath(entry.path)}</span>
							</button>
							<Directory
								directory={entry.path}
								selectedPath={selectedPath}
								expanded={expanded}
								onToggle={onToggle}
								onSelect={onSelect}
							/>
						</div>
					);
				}
				return (
					<button
						key={entry.path}
						type="button"
						role="treeitem"
						aria-selected={selectedPath === entry.path}
						className={cn(
							treeRowClasses,
							selectedPath === entry.path
								? "bg-surface-sky font-medium text-content-link"
								: "text-content-secondary hover:bg-surface-secondary hover:text-content-primary",
						)}
						onClick={() => onSelect(entry)}
					>
						<span className="size-4 shrink-0" aria-hidden="true" />
						<FileTextIcon className="size-4 shrink-0" />
						<span className="truncate">{nameFromPath(entry.path)}</span>
					</button>
				);
			})}
			{query.hasNextPage && (
				<Button
					variant="subtle"
					size="sm"
					className="ml-6 mt-1"
					disabled={query.isFetchingNextPage}
					onClick={() => void query.fetchNextPage()}
				>
					{query.isFetchingNextPage && <Spinner loading size="sm" />}
					Load more
				</Button>
			)}
		</>
	);
	return directory === "/" ? (
		<div role="tree" aria-label="Agent memories" className="space-y-0.5">
			{children}
		</div>
	) : (
		<div
			role="group"
			className="ml-4 space-y-0.5 border-l border-border-default pl-1"
		>
			{children}
		</div>
	);
};

export const AgentMemoryTree: FC<AgentMemoryTreeProps> = (props) => (
	<Directory {...props} directory="/" />
);
