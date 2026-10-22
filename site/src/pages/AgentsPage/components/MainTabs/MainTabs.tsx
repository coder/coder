import { cn } from "cn";
import { useEffect, useEffectEvent, useId, useRef, useState } from "react";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import { useWorkspaceFiles } from "../../context/WorkspaceFilesContext";
import { getPathBasename } from "../../utils/path";
import {
	type FileDraft,
	type FileDraftUpdate,
	FileTab,
} from "../FileTab/FileTab";
import { getTabElementIds, MainTabStrip, type TabKey } from "./MainTabStrip";
import type { MainTabsController } from "./useMainTabs";

type MainTabsProps = {
	mainTabs: MainTabsController;
	/** Opens files without editing, for archived and other users' chats. */
	readOnly: boolean;
	/** The chat transcript and composer, rendered in the pinned Chat tab. */
	children: React.ReactNode;
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
					onActivate={mainTabs.activate}
					onTabKeyDown={handleTabKeyDown}
					onClose={requestCloseFile}
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
				{workspaceFiles &&
					files.map((file, index) => {
						const isActive = file.path === activePath;
						const repoRoot = workspaceFiles.repoRoots.findLast(
							(root) => file.path === root || file.path.startsWith(`${root}/`),
						);
						const ids = getTabElementIds(idPrefix, index);
						return (
							<MainTabPanel
								key={file.path}
								id={ids.panelId}
								role="tabpanel"
								aria-labelledby={ids.tabId}
								isActive={isActive}
								className="absolute inset-0 flex flex-col bg-surface-primary"
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
							</MainTabPanel>
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
					// Back to the tab after Cancel; to the active tab once Discard
					// removed it.
					const target =
						(closeRequest && tabButtonsRef.current.get(closeRequest.path)) ??
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

type MainTabPanelProps = React.ComponentProps<"div"> & {
	isActive: boolean;
};

/**
 * Inactive panels stay mounted and laid out so editors and the transcript
 * keep their state. `invisible` alone does not hide a descendant that sets
 * `visibility: visible`, such as Monaco's find widget, whose `visible` state
 * class matches Tailwind's utility of the same name, so content-visibility
 * also skips rendering the contents. Unlike display:none, it keeps sizes and
 * scroll offsets, so Monaco and the transcript scroller do not relayout.
 */
const MainTabPanel: React.FC<MainTabPanelProps> = ({
	isActive,
	className,
	...props
}) => (
	<div
		{...props}
		className={cn(
			className,
			!isActive && "invisible [content-visibility:hidden]",
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
