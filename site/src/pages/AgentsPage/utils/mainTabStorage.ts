import type { editor } from "monaco-editor";

const mainTabsStorageKeyPrefix = "agents.main-tabs.";

export type PersistedMainTabs = {
	/** Absolute paths of the open file tabs, in tab order. */
	paths: string[];
	/** The active file tab path, or null when the Chat tab is active. */
	activePath: string | null;
};

const emptyMainTabs: PersistedMainTabs = { paths: [], activePath: null };

export function getPersistedMainTabs(chatID: string): PersistedMainTabs {
	const value = localStorage.getItem(`${mainTabsStorageKeyPrefix}${chatID}`);
	if (!value) {
		return emptyMainTabs;
	}

	try {
		const parsed: unknown = JSON.parse(value);
		if (typeof parsed !== "object" || parsed === null) {
			return emptyMainTabs;
		}
		const paths =
			"paths" in parsed && Array.isArray(parsed.paths)
				? [
						...new Set(
							parsed.paths.filter(
								(path): path is string => typeof path === "string",
							),
						),
					]
				: [];
		const activePath =
			"activePath" in parsed &&
			typeof parsed.activePath === "string" &&
			paths.includes(parsed.activePath)
				? parsed.activePath
				: null;
		return { paths, activePath };
	} catch {
		return emptyMainTabs;
	}
}

/** An empty tab list removes the entry so visited chats leave nothing behind. */
export function savePersistedMainTabs(
	chatID: string,
	tabs: PersistedMainTabs,
): void {
	const key = `${mainTabsStorageKeyPrefix}${chatID}`;
	if (tabs.paths.length === 0) {
		localStorage.removeItem(key);
		return;
	}
	localStorage.setItem(key, JSON.stringify(tabs));
}

// Session storage, so a reload restores where the user was in each file
// while a new browser tab starts fresh. Entries outlive their tabs, so a
// file reopened in the same session also returns to where the user left it.
const fileViewStateKey = (chatID: string, path: string) =>
	`agents.file-view-state.${chatID}:${path}`;

/** The editor view state saved for a file in a chat, as Monaco returned it. */
export function getFileViewState(
	chatID: string,
	path: string,
): editor.ICodeEditorViewState | undefined {
	const value = sessionStorage.getItem(fileViewStateKey(chatID, path));
	if (!value) {
		return undefined;
	}
	try {
		const parsed: unknown = JSON.parse(value);
		return typeof parsed === "object" && parsed !== null
			? (parsed as editor.ICodeEditorViewState)
			: undefined;
	} catch {
		return undefined;
	}
}

/** The 1-based line of the cursor saved for a file in a chat. */
export function getFileCursorLine(
	chatID: string,
	path: string,
): number | undefined {
	// The saved view state is parsed without validation.
	const line: unknown = getFileViewState(chatID, path)?.cursorState?.[0]
		?.position?.lineNumber;
	return typeof line === "number" ? line : undefined;
}

export function saveFileViewState(
	chatID: string,
	path: string,
	viewState: editor.ICodeEditorViewState,
): void {
	try {
		sessionStorage.setItem(
			fileViewStateKey(chatID, path),
			JSON.stringify(viewState),
		);
	} catch {
		// A full storage quota only costs the restored position.
	}
}
