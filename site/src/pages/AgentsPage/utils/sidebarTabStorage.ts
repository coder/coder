export const lastActiveSidebarTabStorageKeyPrefix = "agents.last-active-tab.";

export function getPersistedSidebarTabId(chatID: string): string | null {
	return localStorage.getItem(
		`${lastActiveSidebarTabStorageKeyPrefix}${chatID}`,
	);
}

export function savePersistedSidebarTabId(chatID: string, tabID: string): void {
	localStorage.setItem(
		`${lastActiveSidebarTabStorageKeyPrefix}${chatID}`,
		tabID,
	);
}

export function clearPersistedSidebarTabId(chatID: string): void {
	localStorage.removeItem(`${lastActiveSidebarTabStorageKeyPrefix}${chatID}`);
}
