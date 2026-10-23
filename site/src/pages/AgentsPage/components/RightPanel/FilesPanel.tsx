import { cn } from "cn";
import {
	ChevronRightIcon,
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
import { FileIcon } from "#/components/FileIcon/FileIcon";
import { Input } from "#/components/Input/Input";
import { Loader } from "#/components/Loader/Loader";
import {
	useWorkspaceFiles,
	type WorkspaceFilesContextValue,
} from "../../context/WorkspaceFilesContext";
import { getPathBasename, getPathDirname } from "../../utils/path";
import { FilesRootPicker } from "./FilesRootPicker";

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
	const rootTriggerRef = useRef<HTMLButtonElement>(null);
	const pathFormRef = useRef<HTMLFormElement>(null);
	const focusPathInputOnClose = useRef(false);
	// A refresh usually answers in tens of milliseconds, too fast to see, so
	// the button stays busy for at least one turn of the spinner.
	const [isRefreshShown, setIsRefreshShown] = useState(false);
	const refreshShownTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
	useEffect(() => () => clearTimeout(refreshShownTimer.current), []);

	const rootQuery = useQuery(directoryQuery(agentId, root));
	const rootPath = rootQuery.data?.absolute_path_string;
	// Read-only observers: they expose cached listings of every expanded
	// directory, and of the default directory, without fetching.
	const cachedPaths = [...expandedPaths, ""];
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
	// running, waits and then runs once. Each entry carries a token so a
	// superseded chain cannot clear a newer one.
	const refreshes = useRef(
		new Map<string, { trailing: boolean; token: number }>(),
	);
	const refreshToken = useRef(0);
	const refreshDirectory = (path: string, restart = false) => {
		const current = refreshes.current.get(path);
		if (current && !restart) {
			current.trailing = true;
			return;
		}
		const queryKey = workspaceAgentDirectoryKey(agentId, path);
		refreshToken.current += 1;
		const token = refreshToken.current;
		refreshes.current.set(path, {
			trailing:
				!restart && queryClient.isFetching({ queryKey, exact: true }) > 0,
			token,
		});
		// An explicit refresh aborts a fetch in flight instead of waiting for
		// it, so a stalled request cannot keep stale entries on screen.
		// cancelRefetch alone would join a first load that has no data yet.
		void (
			restart
				? queryClient.cancelQueries({ queryKey, exact: true })
				: Promise.resolve()
		)
			.then(() =>
				queryClient.invalidateQueries(
					{ queryKey, exact: true },
					{ cancelRefetch: false },
				),
			)
			.finally(() => {
				const entry = refreshes.current.get(path);
				if (entry?.token !== token) {
					return;
				}
				refreshes.current.delete(path);
				if (entry.trailing) {
					refreshDirectory(path);
				}
			});
	};

	// The cache observers above keep hidden directories' queries alive, so a
	// fetch started while a directory was shown would otherwise run on after
	// it is collapsed, deleted, or the root changes.
	const shownDirectoriesKey = [root, ...visibleExpandedPaths].join("\n");
	const previousShownDirectories = useRef<readonly string[]>([]);
	useEffect(() => {
		const shown = shownDirectoriesKey.split("\n");
		for (const path of previousShownDirectories.current) {
			if (!shown.includes(path)) {
				void queryClient.cancelQueries({
					queryKey: workspaceAgentDirectoryKey(agentId, path),
					exact: true,
				});
			}
		}
		previousShownDirectories.current = shown;
	}, [agentId, queryClient, shownDirectoriesKey]);

	const refetchDirectories = (
		absolutePaths: readonly string[],
		restart = false,
	) => {
		if (rootPath === undefined || absolutePaths.includes(rootPath)) {
			refreshDirectory(root, restart);
		}
		for (const path of visibleExpandedPaths) {
			if (absolutePaths.includes(path)) {
				refreshDirectory(path, restart);
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
			// The lookup can outlast the user's attention: only a form that still
			// holds focus (or lost it to the body) hands it back to the picker.
			const active = document.activeElement;
			const formHadFocus =
				active === document.body ||
				(active !== null && pathFormRef.current?.contains(active) === true);
			selectRoot(result.root);
			if (result.file) {
				openFile(result.file);
			} else if (formHadFocus) {
				// The path form closes with the selection; keep focus on the picker
				// that opened it rather than the page body.
				rootTriggerRef.current?.focus();
			}
		},
	});

	const cancelOpenPath = () => {
		openPathRequest.current += 1;
		openPathMutation.reset();
	};

	// The root picker abbreviates paths under home and folds the default
	// directory into Home when they are the same place, so it needs home's
	// absolute path.
	const homeQuery = useQuery(workspaceAgentDirectory(agentId, "~"));
	const homePath = homeQuery.data?.absolute_path_string;
	const defaultPath =
		defaultDirectory ?? cachedListing("")?.absolute_path_string;
	const homeIsDefault =
		defaultDirectory === undefined ||
		(homePath !== undefined && homePath === defaultPath);

	const handleSelectRoot = (nextRoot: string) => {
		cancelOpenPath();
		selectRoot(nextRoot);
	};

	const handleChooseOpenPath = () => {
		cancelOpenPath();
		focusPathInputOnClose.current = true;
		setIsEnteringPath(true);
	};

	const handleMenuCloseAutoFocus = (event: Event) => {
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
		setIsRefreshShown(true);
		clearTimeout(refreshShownTimer.current);
		refreshShownTimer.current = setTimeout(
			() => setIsRefreshShown(false),
			refreshFeedbackMs,
		);
		refetchDirectories(
			[...(rootPath === undefined ? [] : [rootPath]), ...visibleExpandedPaths],
			true,
		);
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
			<div className="flex shrink-0 items-center justify-between gap-2 border-0 border-b border-solid border-border p-2">
				<FilesRootPicker
					root={root}
					rootPath={rootPath}
					homePath={homePath}
					defaultDirectory={homeIsDefault ? undefined : defaultDirectory}
					repoRoots={repoRoots}
					triggerRef={rootTriggerRef}
					onSelectRoot={handleSelectRoot}
					onOpenPath={handleChooseOpenPath}
					onCloseAutoFocus={handleMenuCloseAutoFocus}
				/>
				<Button
					variant="subtle"
					size="icon"
					aria-label="Refresh files"
					title="Refresh files"
					aria-busy={isFetching || isRefreshShown}
					className="size-6"
					onClick={handleRefresh}
				>
					<RefreshCwIcon
						className={cn(
							"size-3.5",
							(isFetching || isRefreshShown) && "animate-spin",
						)}
					/>
				</Button>
			</div>
			{isEnteringPath && (
				<form
					ref={pathFormRef}
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
									rootTriggerRef.current?.focus();
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

// Sized to the 14px folder icon so file and folder rows line up.
const fileIconStyle: React.CSSProperties = {
	fontSize: 16,
	height: "0.875rem",
	minWidth: "0.875rem",
};

const typeaheadTimeoutMs = 500;
const refreshFeedbackMs = 1000;

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
	const focusedIndex = useRef(0);
	const typeahead = useRef({ prefix: "", timeStamp: 0 });

	const rows = buildTreeRows(listing.contents, childListings);
	const entryRows = rows.filter((row) => row.type === "entry");
	const tabStopPath = entryRows.some(
		(row) => row.entry.absolute_path_string === focusedPath,
	)
		? focusedPath
		: entryRows[0]?.entry.absolute_path_string;

	// A refresh can drop the focused row, which sends focus to the page body
	// without a blur event. Focus then moves to the row now at its position.
	const treeHasFocus = useRef(false);
	const entryPathsKey = entryRows
		.map((row) => row.entry.absolute_path_string)
		.join("\n");
	useEffect(() => {
		const paths = entryPathsKey === "" ? [] : entryPathsKey.split("\n");
		if (
			!treeHasFocus.current ||
			focusedPath === undefined ||
			paths.includes(focusedPath) ||
			document.activeElement !== document.body
		) {
			return;
		}
		const next = paths[Math.min(focusedIndex.current, paths.length - 1)];
		if (next !== undefined) {
			rowElements.current.get(next)?.focus();
		}
	}, [entryPathsKey, focusedPath]);

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
					onFocus={() => {
						treeHasFocus.current = true;
					}}
					onBlur={() => {
						treeHasFocus.current = false;
					}}
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
										? "text-content-secondary"
										: "text-content-primary",
								)}
								style={{ paddingLeft: `${(row.level - 1) * 12 + 4}px` }}
								onFocus={() => {
									setFocusedPath(path);
									focusedIndex.current = index;
								}}
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
									<FileIcon
										fileName={entry.name}
										className={cn("shrink-0", entry.ignored && "opacity-60")}
										style={fileIconStyle}
									/>
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
