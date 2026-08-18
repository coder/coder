import { chatSidebarTabStorage } from "#/utils/storage/keys";

export function getPersistedSidebarTabId(
	chatID: string | undefined,
): string | null {
	if (!chatID) {
		return null;
	}
	return chatSidebarTabStorage.forId(chatID).get();
}

export function savePersistedSidebarTabId(
	chatID: string | undefined,
	tabID: string,
): void {
	if (!chatID) {
		return;
	}
	chatSidebarTabStorage.forId(chatID).set(tabID);
}
