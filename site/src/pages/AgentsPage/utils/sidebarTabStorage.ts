export const lastActiveSidebarTabStorageKeyPrefix = "agents.last-active-tab.";

export function getPersistedSidebarTabId(
	chatID: string | undefined,
): string | null {
	if (!chatID) {
		return null;
	}
	return localStorage.getItem(
		`${lastActiveSidebarTabStorageKeyPrefix}${chatID}`,
	);
}

export function savePersistedSidebarTabId(
	chatID: string | undefined,
	tabID: string,
): void {
	if (!chatID) {
		return;
	}
	localStorage.setItem(
		`${lastActiveSidebarTabStorageKeyPrefix}${chatID}`,
		tabID,
	);
}

export function clearPersistedSidebarTabId(chatID: string | undefined): void {
	if (!chatID) {
		return;
	}
	localStorage.removeItem(`${lastActiveSidebarTabStorageKeyPrefix}${chatID}`);
}
