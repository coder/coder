import {
	isSingletonRightPanelTabId,
	isUserRightPanelTab,
	type SingletonRightPanelTabId,
	singletonRightPanelTabIds,
	type UserRightPanelTab,
} from "./rightPanelTabs";

export const rightPanelTabStorageKeyPrefix = "agents.right-panel-tabs.";

export function getPersistedRightPanelTabs(
	chatID: string | undefined,
): UserRightPanelTab[] {
	if (!chatID) {
		return [];
	}

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
	chatID: string | undefined,
	tabs: readonly UserRightPanelTab[],
): void {
	if (!chatID) {
		return;
	}
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
	chatID: string | undefined,
): SingletonRightPanelTabId[] {
	if (!chatID) {
		return [];
	}

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
	chatID: string | undefined,
	tabIds: readonly SingletonRightPanelTabId[],
): void {
	if (!chatID) {
		return;
	}
	localStorage.setItem(
		`${visibleSingletonTabsStorageKeyPrefix}${chatID}`,
		JSON.stringify(tabIds),
	);
}

const defaultTerminalHiddenStorageKeyPrefix = "agents.default-terminal-hidden.";

export const dismissedMcpAppStorageKeyPrefix =
	"agents.right-panel-mcp-app-dismissed.";

/**
 * App tabs the user closed, keyed by tab ID with the tool call each one was
 * showing when closed.
 */
export function getPersistedDismissedMcpAppTabs(
	chatID: string | undefined,
): Map<string, string> {
	if (!chatID) {
		return new Map();
	}
	const value = localStorage.getItem(
		`${dismissedMcpAppStorageKeyPrefix}${chatID}`,
	);
	if (!value) {
		return new Map();
	}
	try {
		const parsed: unknown = JSON.parse(value);
		if (
			typeof parsed !== "object" ||
			parsed === null ||
			Array.isArray(parsed)
		) {
			return new Map();
		}
		return new Map(
			Object.entries(parsed).filter(
				(entry): entry is [string, string] => typeof entry[1] === "string",
			),
		);
	} catch {
		return new Map();
	}
}

export function savePersistedDismissedMcpAppTabs(
	chatID: string | undefined,
	dismissed: ReadonlyMap<string, string>,
): void {
	if (!chatID) {
		return;
	}
	const key = `${dismissedMcpAppStorageKeyPrefix}${chatID}`;
	if (dismissed.size === 0) {
		localStorage.removeItem(key);
		return;
	}
	localStorage.setItem(key, JSON.stringify(Object.fromEntries(dismissed)));
}

export function getPersistedDefaultTerminalHidden(
	chatID: string | undefined,
): boolean {
	if (!chatID) {
		return false;
	}
	return (
		localStorage.getItem(
			`${defaultTerminalHiddenStorageKeyPrefix}${chatID}`,
		) === "true"
	);
}

export function savePersistedDefaultTerminalHidden(
	chatID: string | undefined,
	hidden: boolean,
): void {
	if (!chatID) {
		return;
	}
	const key = `${defaultTerminalHiddenStorageKeyPrefix}${chatID}`;
	if (hidden) {
		localStorage.setItem(key, "true");
	} else {
		localStorage.removeItem(key);
	}
}

export function clearPersistedRightPanelState(
	chatID: string | undefined,
): void {
	if (!chatID) {
		return;
	}
	localStorage.removeItem(`${rightPanelTabStorageKeyPrefix}${chatID}`);
	localStorage.removeItem(`${visibleSingletonTabsStorageKeyPrefix}${chatID}`);
	localStorage.removeItem(`${defaultTerminalHiddenStorageKeyPrefix}${chatID}`);
	localStorage.removeItem(`${dismissedMcpAppStorageKeyPrefix}${chatID}`);
}
