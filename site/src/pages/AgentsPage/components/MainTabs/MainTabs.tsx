import { cva } from "class-variance-authority";
import { cn } from "cn";
import { XIcon } from "lucide-react";
import { useEffect, useEffectEvent, useId, useRef, useState } from "react";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import { useWorkspaceFiles } from "../../context/WorkspaceFilesContext";
import { getPathBasename, getPathDirname } from "../../utils/path";
import {
	type FileDraft,
	type FileDraftUpdate,
	FileTab,
} from "../FileTab/FileTab";
import type { MainTabsController } from "./useMainTabs";

type MainTabsProps = {
	mainTabs: MainTabsController;
	/** Opens files without editing, for archived and other users' chats. */
	readOnly: boolean;
	/** The chat transcript and composer, rendered in the pinned Chat tab. */
	children: React.ReactNode;
};

/** A tab's path, or null for the Chat tab. */
type TabKey = string | null;

const tabButtonVariants = cva(
	"h-6 min-w-0 shrink-0 gap-1.5 bg-surface-primary px-2 py-0",
	{
		variants: {
			active: {
				true: "bg-surface-quaternary/25 text-content-primary hover:bg-surface-quaternary/50",
				false: "",
			},
		},
	},
);

const trackElement =
	<Key,>(elements: React.RefObject<Map<Key, HTMLButtonElement>>, key: Key) =>
	(element: HTMLButtonElement | null) => {
		if (!element) {
			return;
		}
		elements.current.set(key, element);
		return () => {
			elements.current.delete(key);
		};
	};

/**
 * The chat view's main tabs: a pinned Chat tab followed by file tabs. The
 * chat stays mounted and laid out while a file tab is active, so streaming,
 * the transcript scroll position, and the composer draft survive switching.
 */
export const MainTabs: React.FC<MainTabsProps> = ({
	mainTabs,
	readOnly,
	children,
}) => {
	const workspaceFiles = useWorkspaceFiles();
	const idPrefix = useId();
	const rootRef = useRef<HTMLDivElement>(null);
	// Keyboard navigation and the close confirmation move focus to these.
	const tabButtonsRef = useRef(new Map<TabKey, HTMLButtonElement>());
	const closeButtonsRef = useRef(new Map<string, HTMLButtonElement>());
	// Drafts change on every keystroke, so they live here instead of in the
	// page view to keep typing from re-rendering the whole chat page.
	const [drafts, setDrafts] = useState<ReadonlyMap<string, FileDraft>>(
		() => new Map(),
	);
	// Kept after the dialog closes so its text and the focus return target
	// survive the close animation.
	const [closeRequest, setCloseRequest] = useState<{
		path: string;
		isOpen: boolean;
	}>();

	const files = workspaceFiles ? mainTabs.files : [];
	const activePath = files.some((file) => file.path === mainTabs.activePath)
		? mainTabs.activePath
		: null;
	const hasFileTabs = files.length > 0;
	const isChatActive = activePath === null;
	const tabOrder: TabKey[] = [null, ...files.map((file) => file.path)];

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

	const closeFile = (path: string) => {
		mainTabs.closeFile(path);
		updateDraft(path, () => undefined);
	};

	const requestCloseFile = (path: string) => {
		if (drafts.has(path)) {
			setCloseRequest({ path, isOpen: true });
			return;
		}
		closeFile(path);
	};

	const selectTabAt = (index: number) => {
		const key = tabOrder[(index + tabOrder.length) % tabOrder.length] ?? null;
		mainTabs.activate(key);
		// The panel that had focus turns inert, so focus follows the tab.
		tabButtonsRef.current.get(key)?.focus();
	};

	const handleTabKeyDown = (event: React.KeyboardEvent, key: TabKey) => {
		const index = tabOrder.indexOf(key);
		let next: number;
		switch (event.key) {
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
		// The right panel (a terminal) and dialogs keep these keys.
		const { target } = event;
		const isInScope =
			target === document.body ||
			(target instanceof Node && rootRef.current?.contains(target));
		if (!isInScope) {
			return;
		}
		event.preventDefault();
		event.stopPropagation();
		selectTabAt(tabOrder.indexOf(activePath) + offset);
	});

	useEffect(() => {
		if (!hasFileTabs) {
			return;
		}
		// Capture phase so the shortcut wins over a focused Monaco editor.
		window.addEventListener("keydown", cycleTabs, true);
		return () => window.removeEventListener("keydown", cycleTabs, true);
	}, [hasFileTabs]);

	const basenameCounts = new Map<string, number>();
	for (const file of files) {
		const basename = getPathBasename(file.path);
		basenameCounts.set(basename, (basenameCounts.get(basename) ?? 0) + 1);
	}

	const chatTabId = `${idPrefix}-tab-chat`;
	const chatPanelId = `${idPrefix}-panel-chat`;

	return (
		<div ref={rootRef} className="flex min-h-0 flex-1 flex-col">
			{hasFileTabs && (
				<div
					role="tablist"
					aria-label="Chat and files"
					className="relative z-10 flex shrink-0 items-center gap-1 overflow-x-auto border-0 border-b border-solid border-border bg-surface-primary px-4 py-1.5 scrollbar-none [&::-webkit-scrollbar]:hidden"
				>
					<Button
						ref={trackElement(tabButtonsRef, null)}
						id={chatTabId}
						role="tab"
						aria-selected={isChatActive}
						aria-controls={chatPanelId}
						tabIndex={isChatActive ? 0 : -1}
						variant="outline"
						size="lg"
						className={tabButtonVariants({ active: isChatActive })}
						onClick={() => mainTabs.activate(null)}
						onKeyDown={(event) => handleTabKeyDown(event, null)}
					>
						Chat
					</Button>
					{files.map((file, index) => {
						const isActive = file.path === activePath;
						const isDirty = drafts.has(file.path);
						const label = getPathBasename(file.path);
						const parentHint =
							(basenameCounts.get(label) ?? 0) > 1
								? getPathBasename(getPathDirname(file.path))
								: undefined;
						const name = parentHint ? `${label} ${parentHint}` : label;
						return (
							<div key={file.path} className="flex shrink-0 items-center">
								<Button
									ref={trackElement(tabButtonsRef, file.path)}
									id={`${idPrefix}-tab-${index}`}
									role="tab"
									aria-selected={isActive}
									aria-controls={`${idPrefix}-panel-${index}`}
									aria-label={isDirty ? `${name}, unsaved` : name}
									tabIndex={isActive ? 0 : -1}
									title={file.path}
									variant="outline"
									size="lg"
									className={cn(
										tabButtonVariants({ active: isActive }),
										"rounded-r-none border-r-0 pr-2.5",
									)}
									onClick={() => mainTabs.activate(file.path)}
									onKeyDown={(event) => handleTabKeyDown(event, file.path)}
									onMouseDown={(event) => {
										// Middle-click would otherwise start autoscroll.
										if (event.button === 1) {
											event.preventDefault();
										}
									}}
									onAuxClick={(event) => {
										if (event.button === 1) {
											requestCloseFile(file.path);
										}
									}}
								>
									{isDirty && (
										<span
											aria-hidden
											className="size-1.5 shrink-0 rounded-full bg-content-primary"
										/>
									)}
									<span className="truncate">{label}</span>
									{parentHint && (
										<span className="truncate text-content-secondary">
											{parentHint}
										</span>
									)}
								</Button>
								<Button
									ref={trackElement(closeButtonsRef, file.path)}
									variant="outline"
									size="icon"
									aria-label={`Close ${name}`}
									title={`Close ${file.path}`}
									className={cn(
										"size-6 rounded-l-none rounded-r-md bg-surface-primary p-0 text-content-secondary hover:text-content-primary [&>svg]:size-3",
										isActive &&
											"bg-surface-quaternary/25 text-content-primary hover:bg-surface-quaternary/50",
									)}
									onClick={() => requestCloseFile(file.path)}
								>
									<XIcon />
								</Button>
							</div>
						);
					})}
				</div>
			)}
			<div className="relative flex min-h-0 flex-1 flex-col">
				<div
					id={chatPanelId}
					{...(hasFileTabs && {
						role: "tabpanel",
						"aria-labelledby": chatTabId,
					})}
					// Hidden with visibility, not display, so the transcript keeps its
					// layout and the scroller keeps following output in the background.
					className={cn(
						"flex min-h-0 flex-1 flex-col",
						!isChatActive && "invisible",
					)}
					inert={!isChatActive}
				>
					{children}
				</div>
				{workspaceFiles &&
					files.map((file, index) => {
						const isActive = file.path === activePath;
						const repoRoot = workspaceFiles.repoRoots.findLast(
							(root) => file.path === root || file.path.startsWith(`${root}/`),
						);
						return (
							<div
								key={file.path}
								id={`${idPrefix}-panel-${index}`}
								role="tabpanel"
								aria-labelledby={`${idPrefix}-tab-${index}`}
								className={cn(
									"absolute inset-0 flex flex-col bg-surface-primary",
									!isActive && "invisible",
								)}
								inert={!isActive}
							>
								<FileTab
									agentId={workspaceFiles.agentId}
									path={file.path}
									reveal={file.reveal}
									repository={
										repoRoot === undefined
											? undefined
											: workspaceFiles.repositories.get(repoRoot)
									}
									draft={drafts.get(file.path)}
									onDraftChange={updateDraft}
									readOnly={readOnly}
								/>
							</div>
						);
					})}
			</div>
			<ConfirmDialog
				open={closeRequest?.isOpen ?? false}
				type="delete"
				title="Discard unsaved changes?"
				description={`${closeRequest ? getPathBasename(closeRequest.path) : ""} has unsaved changes. Closing the tab discards them.`}
				confirmText="Discard"
				onConfirm={() => {
					if (closeRequest) {
						closeFile(closeRequest.path);
						setCloseRequest({ ...closeRequest, isOpen: false });
					}
				}}
				onClose={() =>
					setCloseRequest((current) => current && { ...current, isOpen: false })
				}
				onCloseAutoFocus={(event) => {
					// Back to the close button after Cancel; to the active tab once
					// Discard removed it.
					const target =
						(closeRequest && closeButtonsRef.current.get(closeRequest.path)) ??
						tabButtonsRef.current.get(activePath);
					if (target) {
						event.preventDefault();
						target.focus();
					}
					setCloseRequest(undefined);
				}}
			/>
			{drafts.size > 0 && <UnsavedFilesPrompt />}
		</div>
	);
};

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
