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
