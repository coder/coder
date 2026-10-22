import { cn } from "cn";
import { useEffect, useEffectEvent, useId, useRef, useState } from "react";
import { flushSync } from "react-dom";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { Loader } from "#/components/Loader/Loader";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import { useWorkspaceFiles } from "../../context/WorkspaceFilesContext";
import { getPathBasename } from "../../utils/path";
import type { ChatStore } from "../ChatConversation/chatStore";
import {
	type FileDraft,
	type FileDraftUpdate,
	FileTab,
} from "../FileTab/FileTab";
import { getTabElementIds, MainTabStrip, type TabKey } from "./MainTabStrip";
import { findRepoRoot, useFileTabRefresh } from "./useFileTabRefresh";
import type { MainTabsController } from "./useMainTabs";

type MainTabsProps = {
	mainTabs: MainTabsController;
	/** The chat's store, whose file tool calls refresh the files they write. */
	store: ChatStore;
	/** Opens files without editing, for archived and other users' chats. */
	readOnly: boolean;
	/**
	 * Whether the chat can open files. File tabs then show from the start,
	 * and their content once the workspace agent connects.
	 */
	filesEnabled: boolean;
	/** The chat transcript and composer, rendered in the pinned Chat tab. */
	children: React.ReactNode;
	/** Moves focus to the composer in `children`. */
	focusChatInput: () => void;
};

/**
 * The chat view's main tabs: a pinned Chat tab followed by file tabs. The
 * chat stays mounted and laid out while a file tab is active, so streaming,
 * the transcript scroll position, and the composer draft survive switching.
 */
export const MainTabs: React.FC<MainTabsProps> = ({
	mainTabs,
	store,
	readOnly,
	filesEnabled,
	children,
	focusChatInput,
}) => {
	const workspaceFiles = useWorkspaceFiles();
	const idPrefix = useId();
	// Keyboard navigation and the close confirmation move focus to these.
	const tabButtonsRef = useRef(new Map<TabKey, HTMLButtonElement>());
	// Drafts change on every keystroke, so they live here instead of in the
	// page view to keep typing from re-rendering the whole chat page.
	const [drafts, setDrafts] = useState<ReadonlyMap<string, FileDraft>>(
		() => new Map(),
	);
	// Kept after the dialog closes so its text and the focus return target
	// survive the close animation.
	const [closeRequest, setCloseRequest] = useState<{
		paths: readonly string[];
		unsavedPaths: readonly string[];
		/** The tab that asked to close them, which Cancel focuses again. */
		source: string;
		isOpen: boolean;
		isConfirmed?: boolean;
	}>();

	const files = filesEnabled ? mainTabs.files : [];
	const activePath = files.some((file) => file.path === mainTabs.activePath)
		? mainTabs.activePath
		: null;
	const hasFileTabs = files.length > 0;
	const isChatActive = activePath === null;
	const tabOrder: TabKey[] = [null, ...files.map((file) => file.path)];

	useFileTabRefresh({
		store,
		workspaceFiles,
		paths: files.map((file) => file.path),
	});

	const updateDraft = (path: string, update: FileDraftUpdate) => {
		setDrafts((current) => {
			const previous = current.get(path);
			const next = update(previous);
			if (next === previous) {
				return current;
			}
			const nextDrafts = new Map(current);
			if (next === undefined) {
				nextDrafts.delete(path);
			} else {
				nextDrafts.set(path, next);
			}
			return nextDrafts;
		});
	};

	/** Activates a tab and moves focus into its content. */
	const focusTab = (key: TabKey) => {
		if (key !== null) {
			mainTabs.activate(key, { focus: true });
			return;
		}
		// The composer cannot take focus until its panel is no longer inert.
		flushSync(() => mainTabs.activate(null));
		focusChatInput();
	};

	const closeFiles = (paths: readonly string[]) => {
		const update = () => {
			mainTabs.closeFiles(paths);
			setDrafts((current) => {
				if (!paths.some((path) => current.has(path))) {
					return current;
				}
				const nextDrafts = new Map(current);
				for (const path of paths) {
					nextDrafts.delete(path);
				}
				return nextDrafts;
			});
		};
		// Closing the active tab focuses the next active tab's content. A file
		// tab focuses itself through its reveal; the Chat tab becomes active
		// only once every file tab closes.
		const closesActive = activePath !== null && paths.includes(activePath);
		if (closesActive && files.every((file) => paths.includes(file.path))) {
			flushSync(update);
			focusChatInput();
			return;
		}
		update();
	};

	const confirmCloseFiles = (
		paths: readonly string[],
		unsavedPaths: readonly string[],
		source: string,
	) => {
		if (unsavedPaths.length > 0) {
			setCloseRequest({ paths, unsavedPaths, source, isOpen: true });
			return;
		}
		closeFiles(paths);
	};

	const requestCloseFiles = (paths: readonly string[], source: string) => {
		confirmCloseFiles(
			paths,
			paths.filter((path) => drafts.has(path)),
			source,
		);
	};

	const requestCloseFile = (path: string) => requestCloseFiles([path], path);

	// Not built on requestCloseFile: file tabs are memoized, and a callback
	// that reads drafts would change, and re-render every tab, on each
	// keystroke.
	const closeFileTab = (path: string, hasUnsavedEdits: boolean) =>
		confirmCloseFiles([path], hasUnsavedEdits ? [path] : [], path);

	const tabAt = (index: number) =>
		tabOrder[(index + tabOrder.length) % tabOrder.length] ?? null;

	const selectTabAt = (index: number) => {
		const key = tabAt(index);
		mainTabs.activate(key);
		// The panel that had focus turns inert, so focus follows the tab.
		tabButtonsRef.current.get(key)?.focus();
	};

	const handleTabKeyDown = (event: React.KeyboardEvent, key: TabKey) => {
		const index = tabOrder.indexOf(key);
		let next: number;
		switch (event.key) {
			case "Delete":
				if (key !== null) {
					event.preventDefault();
					requestCloseFile(key);
				}
				return;
			case "ArrowLeft":
				next = index - 1;
				break;
			case "ArrowRight":
				next = index + 1;
				break;
			case "Home":
				next = 0;
				break;
			case "End":
				next = tabOrder.length - 1;
				break;
			default:
				return;
		}
		event.preventDefault();
		selectTabAt(next);
	};

	const cycleTabs = useEffectEvent((event: KeyboardEvent) => {
		// Match the physical key: macOS Option+[ reports "“" as event.key.
		if (!event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) {
			return;
		}
		let offset: number;
		if (event.code === "BracketLeft") {
			offset = -1;
		} else if (event.code === "BracketRight") {
			offset = 1;
		} else {
			return;
		}
		// Terminals, iframes, and dialogs keep these keys. Keys pressed inside
		// an iframe never reach this window.
		const { target } = event;
		if (
			target instanceof Element &&
			target.closest(".xterm, [role=dialog], [role=alertdialog]")
		) {
			return;
		}
		event.preventDefault();
		event.stopPropagation();
		focusTab(tabAt(tabOrder.indexOf(activePath) + offset));
	});

	useEffect(() => {
		if (!hasFileTabs) {
			return;
		}
		// Capture phase so the shortcut wins over a focused Monaco editor.
		window.addEventListener("keydown", cycleTabs, true);
		return () => window.removeEventListener("keydown", cycleTabs, true);
	}, [hasFileTabs]);

	const chatIds = getTabElementIds(idPrefix);

	return (
		<div className="flex min-h-0 flex-1 flex-col">
			{hasFileTabs && (
				<MainTabStrip
					idPrefix={idPrefix}
					paths={files.map((file) => file.path)}
					activePath={activePath}
					activation={mainTabs.activation}
					dirtyPaths={new Set(drafts.keys())}
					tabButtonsRef={tabButtonsRef}
					getRelativePath={(path) => {
						const root =
							workspaceFiles && findRepoRoot(workspaceFiles.repoRoots, path);
						return !root || root === path
							? undefined
							: path.slice(root.length + 1);
					}}
					onActivate={mainTabs.activate}
					onTabKeyDown={handleTabKeyDown}
					onClose={requestCloseFiles}
				/>
			)}
			<div className="relative flex min-h-0 flex-1 flex-col">
				<MainTabPanel
					id={chatIds.panelId}
					{...(hasFileTabs && {
						role: "tabpanel",
						"aria-labelledby": chatIds.tabId,
					})}
					isActive={isChatActive}
					className="flex min-h-0 flex-1 flex-col"
				>
					{children}
				</MainTabPanel>
				{files.map((file, index) => {
					const isActive = file.path === activePath;
					const ids = getTabElementIds(idPrefix, index);
					return (
						<MainTabPanel
							key={file.path}
							id={ids.panelId}
							role="tabpanel"
							aria-labelledby={ids.tabId}
							isActive={isActive}
							skipsHiddenContent
							className="absolute inset-0 flex flex-col bg-surface-primary"
						>
							{workspaceFiles ? (
								<FileTab
									chatId={mainTabs.chatId}
									agentId={workspaceFiles.agentId}
									path={file.path}
									reveal={file.reveal}
									isActive={isActive}
									draft={drafts.get(file.path)}
									onDraftChange={updateDraft}
									onClose={closeFileTab}
									readOnly={readOnly}
								/>
							) : (
								<Loader label="Waiting for the workspace agent" />
							)}
						</MainTabPanel>
					);
				})}
			</div>
			<ConfirmDialog
				open={closeRequest?.isOpen ?? false}
				type="delete"
				title="Discard unsaved changes?"
				description={
					closeRequest && closeRequest.unsavedPaths.length > 1
						? `${closeRequest.unsavedPaths.length} files have unsaved changes. Closing their tabs discards them.`
						: `${getPathBasename(closeRequest?.unsavedPaths[0] ?? "")} has unsaved changes. Closing the tab discards them.`
				}
				confirmText="Discard"
				onConfirm={() => {
					if (closeRequest) {
						closeFiles(closeRequest.paths);
						setCloseRequest({
							...closeRequest,
							isOpen: false,
							isConfirmed: true,
						});
					}
				}}
				onClose={() =>
					setCloseRequest((current) => current && { ...current, isOpen: false })
				}
				onCloseAutoFocus={(event) => {
					// The dialog traps focus until now, so focus moves to the active
					// tab's content only here after Discard. Cancel returns it to the
					// tab that asked to close.
					event.preventDefault();
					if (closeRequest?.isConfirmed) {
						focusTab(activePath);
					} else if (closeRequest) {
						tabButtonsRef.current.get(closeRequest.source)?.focus();
					}
					setCloseRequest(undefined);
				}}
			/>
			{drafts.size > 0 && <UnsavedFilesPrompt />}
		</div>
	);
};

type MainTabPanelProps = React.ComponentProps<"div"> & {
	isActive: boolean;
	/**
	 * Also skip rendering the contents while inactive. Not for the chat:
	 * content-visibility withholds ResizeObserver callbacks inside, and the
	 * transcript scroller needs them to resume following a reply that
	 * outgrows the space below its anchored turn.
	 */
	skipsHiddenContent?: boolean;
};

/**
 * Inactive panels stay mounted and laid out so editors and the transcript
 * keep their state. Unlike display:none, hiding keeps sizes and scroll
 * offsets, so Monaco and the transcript scroller do not relayout.
 */
const MainTabPanel: React.FC<MainTabPanelProps> = ({
	isActive,
	skipsHiddenContent = false,
	className,
	...props
}) => (
	<div
		{...props}
		className={cn(
			className,
			// A descendant that sets `visibility: visible`, such as Tailwind's
			// `visible` on a control that touch screens show without hover,
			// would show through `invisible` alone.
			!isActive && "invisible [&_*]:invisible!",
			!isActive && skipsHiddenContent && "[content-visibility:hidden]",
		)}
		inert={!isActive}
	/>
);

/**
 * Mounted only while a file is dirty: React Router consults only the newest
 * blocker, so an always-mounted one would override the embed page's.
 */
const UnsavedFilesPrompt: React.FC = () => {
	const unsavedChanges = useUnsavedChangesPrompt(true);
	return (
		<ConfirmDialog
			type="info"
			hideCancel={false}
			open={unsavedChanges.isOpen}
			onClose={unsavedChanges.onCancel}
			onConfirm={unsavedChanges.onConfirm}
			title="Unsaved changes"
			confirmText="Leave"
			description="Open files have unsaved changes. Leave anyway?"
		/>
	);
};
