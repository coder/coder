import { cn } from "cn";
import {
	ChevronRightIcon,
	FileIcon,
	FolderIcon,
	LinkIcon,
	RefreshCwIcon,
} from "lucide-react";
import { useRef, useState } from "react";
import { useQueries, useQuery, useQueryClient } from "react-query";
import type {
	WorkspaceAgentFile,
	WorkspaceAgentListDirectoryResponse,
} from "#/api/api";
import { getErrorMessage, getErrorStatus } from "#/api/errors";
import {
	workspaceAgentDirectory,
	workspaceAgentDirectoryKey,
} from "#/api/queries/workspaceFiles";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Loader } from "#/components/Loader/Loader";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import {
	useWorkspaceFiles,
	type WorkspaceFilesContextValue,
} from "../../context/WorkspaceFilesContext";
import { getPathBasename } from "../../utils/path";

export const filesPanelRootStorageKeyPrefix = "agents.files-panel-root.";

// Radix Select reserves the empty string, and "" is the default directory
// path form, so every root value carries a prefix.
const rootValuePrefix = "path:";
const openPathValue = "open-path";

type FilesPanelProps = {
	chatId: string;
};

export const FilesPanel: React.FC<FilesPanelProps> = ({ chatId }) => {
	const workspaceFiles = useWorkspaceFiles();
	if (!workspaceFiles) {
		return (
			<div className="flex h-full items-center justify-center p-4 text-sm text-content-secondary">
				Start the workspace to browse files
			</div>
		);
	}
	return <FilesBrowser chatId={chatId} workspaceFiles={workspaceFiles} />;
};

type FilesBrowserProps = {
	chatId: string;
	workspaceFiles: WorkspaceFilesContextValue;
};

const FilesBrowser: React.FC<FilesBrowserProps> = ({
	chatId,
	workspaceFiles,
}) => {
	const { agentId, defaultDirectory, repoRoots, openFile } = workspaceFiles;
	const queryClient = useQueryClient();
	const storageKey = `${filesPanelRootStorageKeyPrefix}${chatId}`;
	const [storedRoot, setStoredRoot] = useState(() =>
		localStorage.getItem(storageKey),
	);
	const [expandedPaths, setExpandedPaths] = useState<readonly string[]>([]);
	const [isEnteringPath, setIsEnteringPath] = useState(false);
	const root = storedRoot ?? repoRoots[0] ?? "";

	const rootOptions = [
		...repoRoots.map((path) => ({
			root: path,
			label: getPathBasename(path),
			title: path,
		})),
		{
			root: "",
			label: "Default directory",
			title: defaultDirectory ?? "Default directory",
		},
		{ root: "~", label: "Home", title: "~" },
		{ root: "/", label: "/", title: "/" },
	];
	if (!rootOptions.some((option) => option.root === root)) {
		rootOptions.push({ root, label: root, title: root });
	}

	const selectRoot = (nextRoot: string) => {
		setStoredRoot(nextRoot);
		setExpandedPaths([]);
		setIsEnteringPath(false);
		localStorage.setItem(storageKey, nextRoot);
	};

	const handleRootValueChange = (value: string) => {
		if (value === openPathValue) {
			setIsEnteringPath(true);
			return;
		}
		selectRoot(value.slice(rootValuePrefix.length));
	};

	const handleOpenPath = (event: React.FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		const path = new FormData(event.currentTarget).get("path");
		if (typeof path === "string" && path.trim() !== "") {
			selectRoot(path.trim());
		}
	};

	const toggleDirectory = (path: string) => {
		// Collapsing drops expanded descendants so their listings stop being
		// observed and refreshed while hidden.
		setExpandedPaths((paths) =>
			paths.includes(path)
				? paths.filter((p) => p !== path && !p.startsWith(`${path}/`))
				: [...paths, path],
		);
	};

	const handleRefresh = () => {
		for (const path of [root, ...expandedPaths]) {
			void queryClient.invalidateQueries({
				queryKey: workspaceAgentDirectoryKey(agentId, path),
			});
		}
	};

	return (
		<div className="flex h-full min-h-0 flex-col">
			<div className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border p-2">
				<Select
					value={`${rootValuePrefix}${root}`}
					onValueChange={handleRootValueChange}
				>
					<SelectTrigger
						aria-label="Root directory"
						className="h-7 min-w-0 flex-1 text-xs"
					>
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{rootOptions.map((option) => (
							<SelectItem
								key={option.root}
								value={`${rootValuePrefix}${option.root}`}
								title={option.title}
							>
								{option.label}
							</SelectItem>
						))}
						<SelectItem value={openPathValue}>Open path...</SelectItem>
					</SelectContent>
				</Select>
				<Button
					variant="subtle"
					size="icon"
					aria-label="Refresh files"
					title="Refresh files"
					className="size-7"
					onClick={handleRefresh}
				>
					<RefreshCwIcon className="size-3.5" />
				</Button>
			</div>
			{isEnteringPath && (
				<form
					className="flex shrink-0 gap-2 border-0 border-b border-solid border-border p-2"
					onSubmit={handleOpenPath}
				>
					<Input
						name="path"
						aria-label="Path"
						placeholder="/abs/path, ~/path or relative/path"
						className="h-7 text-xs"
						autoFocus
						onKeyDown={(event) => {
							if (event.key === "Escape") {
								setIsEnteringPath(false);
							}
						}}
					/>
					<Button type="submit" size="sm" variant="outline" className="h-7">
						Open
					</Button>
				</form>
			)}
			<DirectoryTree
				agentId={agentId}
				root={root}
				expandedPaths={expandedPaths}
				onToggleDirectory={toggleDirectory}
				onOpenFile={openFile}
			/>
		</div>
	);
};

const directoryErrorMessage = (error: unknown) =>
	getErrorStatus(error) === 404
		? "Directory not found."
		: getErrorMessage(error, "Failed to list the directory.");

type TreeRow =
	| {
			type: "entry";
			entry: WorkspaceAgentFile;
			index: number;
			level: number;
			parentPath: string | undefined;
	  }
	| { type: "status"; key: string; level: number; label: string };

type ChildListing = {
	data: WorkspaceAgentListDirectoryResponse | undefined;
	isError: boolean;
	error: unknown;
};

const buildTreeRows = (
	rootEntries: readonly WorkspaceAgentFile[],
	childListings: ReadonlyMap<string, ChildListing>,
): TreeRow[] => {
	const rows: TreeRow[] = [];
	let entryCount = 0;
	const addEntries = (
		entries: readonly WorkspaceAgentFile[],
		level: number,
		parentPath: string | undefined,
	) => {
		for (const entry of entries) {
			const path = entry.absolute_path_string;
			rows.push({
				type: "entry",
				entry,
				index: entryCount++,
				level,
				parentPath,
			});
			const child = entry.is_dir ? childListings.get(path) : undefined;
			if (!child) {
				continue;
			}
			const childLevel = level + 1;
			if (child.data === undefined) {
				rows.push({
					type: "status",
					key: `${path}:load`,
					level: childLevel,
					label: child.isError
						? directoryErrorMessage(child.error)
						: "Loading...",
				});
				continue;
			}
			if (child.isError) {
				rows.push({
					type: "status",
					key: `${path}:refetch`,
					level: childLevel,
					label: `Refresh failed: ${directoryErrorMessage(child.error)}`,
				});
			}
			if (child.data.contents.length === 0) {
				rows.push({
					type: "status",
					key: `${path}:empty`,
					level: childLevel,
					label: "Empty",
				});
				continue;
			}
			addEntries(child.data.contents, childLevel, path);
			if (child.data.truncated) {
				rows.push({
					type: "status",
					key: `${path}:truncated`,
					level: childLevel,
					label: "Some entries are not shown.",
				});
			}
		}
	};
	addEntries(rootEntries, 1, undefined);
	return rows;
};

type DirectoryTreeProps = {
	agentId: string;
	root: string;
	expandedPaths: readonly string[];
	onToggleDirectory: (path: string) => void;
	onOpenFile: (path: string) => void;
};

const DirectoryTree: React.FC<DirectoryTreeProps> = ({
	agentId,
	root,
	expandedPaths,
	onToggleDirectory,
	onOpenFile,
}) => {
	const rootQuery = useQuery(workspaceAgentDirectory(agentId, root));
	const childListings = useQueries({
		queries: expandedPaths.map((path) =>
			workspaceAgentDirectory(agentId, path),
		),
	});
	const [focusedPath, setFocusedPath] = useState<string>();
	const rowElements = useRef(new Map<string, HTMLDivElement>());

	if (rootQuery.data === undefined) {
		if (!rootQuery.isError) {
			return <Loader className="flex-1" />;
		}
		return (
			<div className="p-2">
				{getErrorStatus(rootQuery.error) === 404 ? (
					<p className="m-0 text-sm text-content-secondary">
						No directory at {root || "the default directory"}.
					</p>
				) : (
					<ErrorAlert error={rootQuery.error} showDebugDetail={false} />
				)}
			</div>
		);
	}

	const listing = rootQuery.data;
	const rows = buildTreeRows(
		listing.contents,
		new Map(expandedPaths.map((path, index) => [path, childListings[index]])),
	);

	const entryRows = rows.filter((row) => row.type === "entry");
	const tabStopPath = entryRows.some(
		(row) => row.entry.absolute_path_string === focusedPath,
	)
		? focusedPath
		: entryRows[0]?.entry.absolute_path_string;

	const focusPath = (path: string | undefined) => {
		if (path === undefined) {
			return;
		}
		setFocusedPath(path);
		rowElements.current.get(path)?.focus();
	};

	const activate = (entry: WorkspaceAgentFile) => {
		if (entry.is_dir) {
			onToggleDirectory(entry.absolute_path_string);
		} else {
			onOpenFile(entry.absolute_path_string);
		}
	};

	const handleKeyDown = (
		event: React.KeyboardEvent<HTMLDivElement>,
		index: number,
	) => {
		const row = entryRows[index];
		const path = row.entry.absolute_path_string;
		const isExpanded = row.entry.is_dir && expandedPaths.includes(path);
		switch (event.key) {
			case "ArrowDown":
				focusPath(entryRows[index + 1]?.entry.absolute_path_string);
				break;
			case "ArrowUp":
				focusPath(entryRows[index - 1]?.entry.absolute_path_string);
				break;
			case "Home":
				focusPath(entryRows[0]?.entry.absolute_path_string);
				break;
			case "End":
				focusPath(entryRows.at(-1)?.entry.absolute_path_string);
				break;
			case "ArrowRight":
				if (!row.entry.is_dir) {
					return;
				}
				if (!isExpanded) {
					onToggleDirectory(path);
				} else if (entryRows[index + 1]?.parentPath === path) {
					focusPath(entryRows[index + 1].entry.absolute_path_string);
				}
				break;
			case "ArrowLeft":
				if (isExpanded) {
					onToggleDirectory(path);
				} else {
					focusPath(row.parentPath);
				}
				break;
			case "Enter":
				activate(row.entry);
				break;
			default:
				return;
		}
		event.preventDefault();
	};

	return (
		<div className="flex min-h-0 flex-1 flex-col">
			<div
				className="shrink-0 truncate px-3 pt-2 text-xs text-content-secondary"
				title={listing.absolute_path_string}
			>
				{listing.absolute_path_string}
			</div>
			{listing.truncated && (
				<p className="m-0 shrink-0 px-3 pt-1 text-xs text-content-warning">
					This directory has more entries than the agent lists.
				</p>
			)}
			{rootQuery.isError && (
				<p
					role="alert"
					className="m-0 shrink-0 px-3 pt-1 text-xs text-content-destructive"
				>
					Refresh failed: {directoryErrorMessage(rootQuery.error)}
				</p>
			)}
			{listing.contents.length === 0 ? (
				<p className="m-0 p-3 text-sm text-content-secondary">
					This directory is empty.
				</p>
			) : (
				<div
					role="tree"
					aria-label="Files"
					className="min-h-0 flex-1 overflow-auto p-1 text-xs"
				>
					{rows.map((row) => {
						if (row.type === "status") {
							return (
								<div
									key={`status:${row.key}`}
									role="treeitem"
									aria-level={row.level}
									aria-disabled
									tabIndex={-1}
									className="py-1 pr-2 text-content-secondary"
									style={{ paddingLeft: `${row.level * 12 + 8}px` }}
								>
									{row.label}
								</div>
							);
						}
						const { entry } = row;
						const path = entry.absolute_path_string;
						const isExpanded = entry.is_dir && expandedPaths.includes(path);
						const { index } = row;
						return (
							<div
								key={path}
								ref={(element) => {
									if (element) {
										rowElements.current.set(path, element);
									} else {
										rowElements.current.delete(path);
									}
								}}
								role="treeitem"
								aria-level={row.level}
								aria-expanded={entry.is_dir ? isExpanded : undefined}
								tabIndex={path === tabStopPath ? 0 : -1}
								className={cn(
									"flex cursor-pointer items-center gap-1 rounded-sm py-1 pr-2 text-content-primary outline-none",
									"hover:bg-surface-secondary focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link",
								)}
								style={{ paddingLeft: `${(row.level - 1) * 12 + 4}px` }}
								onFocus={() => setFocusedPath(path)}
								onClick={() => activate(entry)}
								onKeyDown={(event) => handleKeyDown(event, index)}
							>
								{entry.is_dir ? (
									<ChevronRightIcon
										aria-hidden
										className={cn(
											"size-3 shrink-0 text-content-secondary transition-transform",
											isExpanded && "rotate-90",
										)}
									/>
								) : (
									<span aria-hidden className="size-3 shrink-0" />
								)}
								{entry.is_dir ? (
									<FolderIcon aria-hidden className="size-3.5 shrink-0" />
								) : (
									<FileIcon aria-hidden className="size-3.5 shrink-0" />
								)}
								<span className="truncate">{entry.name}</span>
								{entry.is_symlink && (
									<LinkIcon
										className="size-3 shrink-0 text-content-secondary"
										aria-label="symbolic link"
										role="img"
									/>
								)}
							</div>
						);
					})}
				</div>
			)}
		</div>
	);
};
