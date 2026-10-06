import {
	isSingletonRightPanelTabId,
	isUserRightPanelTab,
	type SingletonRightPanelTabId,
	singletonRightPanelTabIds,
	type UserRightPanelTab,
} from "./rightPanelTabs";

export const rightPanelTabStorageKeyPrefix = "agents.right-panel-tabs.";

export function getPersistedRightPanelTabs(
	chatID: string,
): UserRightPanelTab[] {
	const value = localStorage.getItem(
		`${rightPanelTabStorageKeyPrefix}${chatID}`,
	);
	if (!value) {
		return [];
	}

	try {
		const parsed: unknown = JSON.parse(value);
		if (!Array.isArray(parsed)) {
			return [];
		}
		return parsed.filter(isUserRightPanelTab);
	} catch {
		return [];
	}
}

export function savePersistedRightPanelTabs(
	chatID: string,
	tabs: readonly UserRightPanelTab[],
): void {
	localStorage.setItem(
		`${rightPanelTabStorageKeyPrefix}${chatID}`,
		JSON.stringify(tabs),
	);
}

export const visibleSingletonTabsStorageKeyPrefix =
	"agents.right-panel-singleton-tabs.";

/**
 * Singleton panels start hidden, so an absent or unreadable entry means no
 * singleton tab is shown.
 */
export function getPersistedVisibleSingletonTabs(
	chatID: string,
): SingletonRightPanelTabId[] {
	const value = localStorage.getItem(
		`${visibleSingletonTabsStorageKeyPrefix}${chatID}`,
	);
	if (!value) {
		return [];
	}

	try {
		const parsed: unknown = JSON.parse(value);
		if (!Array.isArray(parsed)) {
			return [];
		}
		const storedIds = parsed.filter(isSingletonRightPanelTabId);
		// Reading through the canonical list drops duplicates and keeps a
		// stable order regardless of the order the user enabled the panels.
		return singletonRightPanelTabIds.filter((id) => storedIds.includes(id));
	} catch {
		return [];
	}
}

export function savePersistedVisibleSingletonTabs(
	chatID: string,
	tabIds: readonly SingletonRightPanelTabId[],
): void {
	localStorage.setItem(
		`${visibleSingletonTabsStorageKeyPrefix}${chatID}`,
		JSON.stringify(tabIds),
	);
}

const defaultTerminalHiddenStorageKeyPrefix = "agents.default-terminal-hidden.";

export function getPersistedDefaultTerminalHidden(chatID: string): boolean {
	return (
		localStorage.getItem(
			`${defaultTerminalHiddenStorageKeyPrefix}${chatID}`,
		) === "true"
	);
}

export function savePersistedDefaultTerminalHidden(
	chatID: string,
	hidden: boolean,
): void {
	const key = `${defaultTerminalHiddenStorageKeyPrefix}${chatID}`;
	if (hidden) {
		localStorage.setItem(key, "true");
	} else {
		localStorage.removeItem(key);
	}
}

export function clearPersistedRightPanelState(chatID: string): void {
	localStorage.removeItem(`${rightPanelTabStorageKeyPrefix}${chatID}`);
	localStorage.removeItem(`${visibleSingletonTabsStorageKeyPrefix}${chatID}`);
	localStorage.removeItem(`${defaultTerminalHiddenStorageKeyPrefix}${chatID}`);
}
