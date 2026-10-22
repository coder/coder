import { cn } from "cn";
import {
	ChevronRightIcon,
	FileIcon,
	FolderIcon,
	LinkIcon,
	RefreshCwIcon,
} from "lucide-react";
import {
	useEffect,
	useEffectEvent,
	useRef,
	useState,
	useSyncExternalStore,
} from "react";
import { useMutation, useQueries, useQuery, useQueryClient } from "react-query";
import type {
	WorkspaceAgentFile,
	WorkspaceAgentListDirectoryResponse,
} from "#/api/api";
import { getErrorMessage, getErrorStatus } from "#/api/errors";
import {
	workspaceAgentDirectory,
	workspaceAgentDirectoryKey,
} from "#/api/queries/workspaceFiles";
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
import { getPathBasename, getPathDirname } from "../../utils/path";

export const filesPanelRootStorageKeyPrefix = "agents.files-panel-root.";

const rootListeners = new Set<() => void>();

const subscribeToRoot = (listener: () => void) => {
	rootListeners.add(listener);
	window.addEventListener("storage", listener);
	return () => {
		rootListeners.delete(listener);
		window.removeEventListener("storage", listener);
	};
};

/**
 * The Files panel root for a chat, in any list-directory path form. Quick
 * open reads it too, so a root picked in the panel is also searched.
 */
export const useFilesPanelRoot = (
	chatId: string | undefined,
	repoRoots: readonly string[],
) => {
	const storageKey = `${filesPanelRootStorageKeyPrefix}${chatId}`;
	const storedRoot = useSyncExternalStore(subscribeToRoot, () =>
		chatId === undefined ? null : localStorage.getItem(storageKey),
	);
	const setRoot = (root: string) => {
		localStorage.setItem(storageKey, root);
		for (const listener of rootListeners) {
			listener();
		}
	};
	return { root: storedRoot ?? repoRoots[0] ?? "", setRoot };
};

// Radix Select reserves the empty string, and "" is the default directory
// path form, so every root value carries a prefix.
const rootValuePrefix = "path:";
const openPathValue = "open-path";

// The tree is a live view of files the agent edits, so it refetches when the
// window regains focus even though the app disables that by default.
const directoryQuery = (agentId: string, path: string) => ({
	...workspaceAgentDirectory(agentId, path),
	refetchOnWindowFocus: true,
});

const displayPath = (path: string) => path || "the default directory";

const directoryErrorMessage = (error: unknown, path: string) => {
	switch (getErrorStatus(error)) {
		case 400:
			return `${displayPath(path)} is a file`;
		case 403:
			return `Permission denied: ${displayPath(path)}`;
		case 404:
			return `No directory at ${displayPath(path)}`;
		default:
			return getErrorMessage(error, "Failed to list the directory.");
	}
};

const isWithin = (path: string, directory: string) =>
	path === directory || path.startsWith(`${directory}/`);

/**
 * Expanded directories the current listings show: each one's parent listing
 * contains it, up to the root. Deleted or moved directories drop out once
 * their parent refetches.
 */
const getVisibleExpandedPaths = (
	rootListing: WorkspaceAgentListDirectoryResponse,
	expandedPaths: readonly string[],
	cachedListings: ReadonlyMap<
		string,
		WorkspaceAgentListDirectoryResponse | undefined
	>,
): string[] => {
	const expanded = new Set(expandedPaths);
	const visible: string[] = [];
	const visit = (listing: WorkspaceAgentListDirectoryResponse | undefined) => {
		for (const entry of listing?.contents ?? []) {
			const path = entry.absolute_path_string;
			if (entry.is_dir && expanded.has(path)) {
				visible.push(path);
				visit(cachedListings.get(path));
			}
		}
	};
	visit(rootListing);
	return visible;
};

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
	const { agentId, defaultDirectory, repoRoots, repositories, openFile } =
		workspaceFiles;
	const queryClient = useQueryClient();
	const { root, setRoot } = useFilesPanelRoot(chatId, repoRoots);
	// Kept on collapse so reopening a folder restores its open subfolders.
	const [expandedPaths, setExpandedPaths] = useState<readonly string[]>([]);
	const [isEnteringPath, setIsEnteringPath] = useState(false);
	const pathInputRef = useRef<HTMLInputElement>(null);
	const focusPathInputOnClose = useRef(false);

	const rootQuery = useQuery(directoryQuery(agentId, root));
	const rootPath = rootQuery.data?.absolute_path_string;
	// Read-only observers: they expose cached listings of every expanded
	// directory, and of the default directory and home, without fetching.
	const cachedPaths = [...expandedPaths, "", "~"];
	const cachedListings = useQueries({
		queries: cachedPaths.map((path) => ({
			...workspaceAgentDirectory(agentId, path),
			enabled: false,
		})),
	});
	const cachedListing = (path: string) =>
		cachedListings[cachedPaths.indexOf(path)]?.data;
	const visibleExpandedPaths =
		rootQuery.data === undefined
			? []
			: getVisibleExpandedPaths(
					rootQuery.data,
					expandedPaths,
					new Map(expandedPaths.map((path) => [path, cachedListing(path)])),
				);
	const childQueries = useQueries({
		queries: visibleExpandedPaths.map((path) => directoryQuery(agentId, path)),
	});
	const isFetching =
		rootQuery.isFetching || childQueries.some((query) => query.isFetching);

	// One request per directory at a time: a refresh asked for while one is
	// running, or while a background fetch that predates the change is
	// running, waits and then runs once.
	const refreshes = useRef(new Map<string, "running" | "trailing">());
	const refreshDirectory = (path: string) => {
		if (refreshes.current.has(path)) {
			refreshes.current.set(path, "trailing");
			return;
		}
		const queryKey = workspaceAgentDirectoryKey(agentId, path);
		refreshes.current.set(
			path,
			queryClient.isFetching({ queryKey, exact: true }) > 0
				? "trailing"
				: "running",
		);
		void queryClient
			.invalidateQueries({ queryKey, exact: true }, { cancelRefetch: false })
			.finally(() => {
				const again = refreshes.current.get(path) === "trailing";
				refreshes.current.delete(path);
				if (again) {
					refreshDirectory(path);
				}
			});
	};

	const refetchDirectories = (absolutePaths: readonly string[]) => {
		if (rootPath === undefined || absolutePaths.includes(rootPath)) {
			refreshDirectory(root);
		}
		for (const path of visibleExpandedPaths) {
			if (absolutePaths.includes(path)) {
				refreshDirectory(path);
			}
		}
	};

	const refetchChangedRepositories = useEffectEvent(
		(changedRepoRoots: readonly string[]) => {
			const shownPaths = [
				...(rootPath === undefined ? [] : [rootPath]),
				...visibleExpandedPaths,
			];
			refetchDirectories(
				shownPaths.filter((path) =>
					changedRepoRoots.some((repoRoot) => isWithin(path, repoRoot)),
				),
			);
		},
	);
	// The git watcher replaces a repository's entry when its working tree
	// changes, which is the only signal that files were written.
	const previousRepositories = useRef(repositories);
	useEffect(() => {
		const previous = previousRepositories.current;
		previousRepositories.current = repositories;
		if (previous === repositories) {
			return;
		}
		const changed = [...repositories]
			.filter(([repoRoot, state]) => previous.get(repoRoot) !== state)
			.map(([repoRoot]) => repoRoot);
		if (changed.length > 0) {
			refetchChangedRepositories(changed);
		}
	}, [repositories]);

	const selectRoot = (nextRoot: string) => {
		setRoot(nextRoot);
		setExpandedPaths([]);
		setIsEnteringPath(false);
	};

	// Bumped by any newer navigation so a slow Open path lookup cannot move
	// the root after the user went elsewhere.
	const openPathRequest = useRef(0);
	const openPathMutation = useMutation({
		mutationFn: async ({
			path,
		}: {
			path: string;
			request: number;
		}): Promise<{ root: string; file?: string }> => {
			try {
				await queryClient.fetchQuery(workspaceAgentDirectory(agentId, path));
				return { root: path };
			} catch (error) {
				// Agents answer a file path with 400, older ones with 500.
				const status = getErrorStatus(error);
				if (status !== 400 && status !== 500) {
					throw error;
				}
				const parent = getPathDirname(path);
				const name = getPathBasename(path);
				const parentListing = await queryClient
					.fetchQuery(workspaceAgentDirectory(agentId, parent))
					.catch(() => undefined);
				const file = parentListing?.contents.find(
					(entry) => entry.name === name && !entry.is_dir,
				);
				if (!file) {
					throw error;
				}
				return { root: parent, file: file.absolute_path_string };
			}
		},
		onSuccess: (result, { request }) => {
			if (request !== openPathRequest.current) {
				return;
			}
			selectRoot(result.root);
			if (result.file) {
				openFile(result.file);
			}
		},
	});

	const cancelOpenPath = () => {
		openPathRequest.current += 1;
		openPathMutation.reset();
	};

	// The agent's default directory is home when it reports none, and the two
	// are also one place once both resolve to the same absolute path.
	const homePath = cachedListing("~")?.absolute_path_string;
	const defaultPath =
		defaultDirectory ?? cachedListing("")?.absolute_path_string;
	const homeIsDefault =
		defaultDirectory === undefined ||
		(homePath !== undefined && homePath === defaultPath);
	const resolvedTitle = (path: string, fallback: string) =>
		root === path && rootPath !== undefined ? rootPath : fallback;
	const rootOptions = [
		...repoRoots.map((path) => ({
			root: path,
			label: getPathBasename(path),
			title: path,
		})),
		...(homeIsDefault
			? [
					{
						root: root === "~" ? "~" : "",
						label: "Default directory (~)",
						title: defaultPath ?? homePath ?? "~",
					},
				]
			: [
					{ root: "", label: "Default directory", title: defaultDirectory },
					{ root: "~", label: "Home", title: homePath ?? "~" },
				]),
		{ root: "/", label: "/", title: "/" },
	];
	if (!rootOptions.some((option) => option.root === root)) {
		rootOptions.push({ root, label: root, title: resolvedTitle(root, root) });
	}

	const handleRootValueChange = (value: string) => {
		cancelOpenPath();
		if (value === openPathValue) {
			focusPathInputOnClose.current = true;
			setIsEnteringPath(true);
			return;
		}
		selectRoot(value.slice(rootValuePrefix.length));
	};

	const handleSelectCloseAutoFocus = (event: Event) => {
		if (!focusPathInputOnClose.current) {
			return;
		}
		focusPathInputOnClose.current = false;
		event.preventDefault();
		pathInputRef.current?.focus();
	};

	const handleOpenPath = (event: React.FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		const path = new FormData(event.currentTarget).get("path");
		if (typeof path !== "string") {
			return;
		}
		const trimmed = path.trim().replace(/(.)\/+$/, "$1");
		if (trimmed !== "") {
			openPathRequest.current += 1;
			openPathMutation.mutate({
				path: trimmed,
				request: openPathRequest.current,
			});
		}
	};

	const toggleDirectory = (path: string) => {
		setExpandedPaths((paths) =>
			paths.includes(path) ? paths.filter((p) => p !== path) : [...paths, path],
		);
	};

	const handleRefresh = () => {
		refetchDirectories([
			...(rootPath === undefined ? [] : [rootPath]),
			...visibleExpandedPaths,
		]);
	};

	let content: React.ReactNode;
	if (rootQuery.data === undefined) {
		content = rootQuery.isError ? (
			<p role="alert" className="m-0 p-3 text-sm text-content-secondary">
				{directoryErrorMessage(rootQuery.error, root)}
			</p>
		) : (
			<Loader className="flex-1" />
		);
	} else {
		content = (
			<DirectoryTree
				listing={rootQuery.data}
				refreshError={
					rootQuery.isError
						? directoryErrorMessage(rootQuery.error, root)
						: undefined
				}
				childListings={
					new Map(
						visibleExpandedPaths.map((path, index) => [
							path,
							childQueries[index],
						]),
					)
				}
				expandedPaths={expandedPaths}
				onToggleDirectory={toggleDirectory}
				onOpenFile={(path) => openFile(path, { focus: false })}
			/>
		);
	}

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
					<SelectContent onCloseAutoFocus={handleSelectCloseAutoFocus}>
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
					aria-busy={isFetching}
					className="size-7"
					onClick={handleRefresh}
				>
					<RefreshCwIcon
						className={cn("size-3.5", isFetching && "animate-spin")}
					/>
				</Button>
			</div>
			{isEnteringPath && (
				<form
					className="flex shrink-0 flex-col gap-1 border-0 border-b border-solid border-border p-2"
					onSubmit={handleOpenPath}
				>
					<div className="flex gap-2">
						<Input
							ref={pathInputRef}
							name="path"
							aria-label="Path"
							placeholder="/abs/path, ~/path or relative/path"
							className="h-7 text-xs"
							onKeyDown={(event) => {
								if (event.key === "Escape") {
									cancelOpenPath();
									setIsEnteringPath(false);
								}
							}}
						/>
						<Button
							type="submit"
							size="sm"
							variant="outline"
							className="h-7"
							disabled={openPathMutation.isPending}
						>
							Open
						</Button>
					</div>
					{openPathMutation.isError && (
						<p role="alert" className="m-0 text-xs text-content-destructive">
							{directoryErrorMessage(
								openPathMutation.error,
								openPathMutation.variables?.path ?? "",
							)}
						</p>
					)}
				</form>
			)}
			{content}
		</div>
	);
};

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
						? directoryErrorMessage(child.error, path)
						: "Loading...",
				});
				continue;
			}
			if (child.isError) {
				rows.push({
					type: "status",
					key: `${path}:refetch`,
					level: childLevel,
					label: `Refresh failed: ${directoryErrorMessage(child.error, path)}`,
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

const typeaheadTimeoutMs = 500;

type DirectoryTreeProps = {
	listing: WorkspaceAgentListDirectoryResponse;
	refreshError: string | undefined;
	childListings: ReadonlyMap<string, ChildListing>;
	expandedPaths: readonly string[];
	onToggleDirectory: (path: string) => void;
	onOpenFile: (path: string) => void;
};

const DirectoryTree: React.FC<DirectoryTreeProps> = ({
	listing,
	refreshError,
	childListings,
	expandedPaths,
	onToggleDirectory,
	onOpenFile,
}) => {
	const [focusedPath, setFocusedPath] = useState<string>();
	const rowElements = useRef(new Map<string, HTMLDivElement>());
	const typeahead = useRef({ prefix: "", timeStamp: 0 });

	const rows = buildTreeRows(listing.contents, childListings);
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

	const jumpToPrefix = (key: string, timeStamp: number, index: number) => {
		const previous = typeahead.current;
		const prefix =
			timeStamp - previous.timeStamp < typeaheadTimeoutMs
				? previous.prefix + key.toLowerCase()
				: key.toLowerCase();
		typeahead.current = { prefix, timeStamp };
		// A repeated first letter cycles; a longer prefix may still match the
		// focused row.
		const start = prefix.length > 1 ? index : index + 1;
		const ordered = [...entryRows.slice(start), ...entryRows.slice(0, start)];
		const match = ordered.find((row) =>
			row.entry.name.toLowerCase().startsWith(prefix),
		);
		focusPath(match?.entry.absolute_path_string);
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
			case " ":
				activate(row.entry);
				break;
			case "Escape":
				// Focus stays on the row, per the WAI-ARIA tree pattern; Escape only
				// ends a type-to-jump prefix.
				typeahead.current = { prefix: "", timeStamp: 0 };
				return;
			default:
				if (
					event.key.length !== 1 ||
					event.ctrlKey ||
					event.metaKey ||
					event.altKey
				) {
					return;
				}
				jumpToPrefix(event.key, event.timeStamp, index);
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
			{refreshError !== undefined && (
				<p
					role="alert"
					className="m-0 shrink-0 px-3 pt-1 text-xs text-content-destructive"
				>
					Refresh failed: {refreshError}
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
						const { entry, index } = row;
						const path = entry.absolute_path_string;
						const isExpanded = entry.is_dir && expandedPaths.includes(path);
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
									"flex cursor-pointer items-center gap-1 rounded-sm py-1 pr-2 outline-none",
									"hover:bg-surface-secondary focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link",
									entry.ignored
										? "text-content-disabled"
										: "text-content-primary",
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
