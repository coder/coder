/**
 * Utilities for persisting and restoring the active sidebar tab per
 * chat. Stored in localStorage keyed by chat ID so a returning user
 * lands on the same tab they last used for that chat.
 */

/** @internal localStorage key prefix for the per-chat active sidebar tab. Exported for testing. */
export const lastActiveSidebarTabStorageKeyPrefix = "agents.last-active-tab.";

/**
 * Read the persisted active sidebar tab ID for a given chat. Returns
 * `null` when no value is stored or the chat ID is missing.
 */
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

/**
 * Persist the active sidebar tab ID for a given chat so it can be
 * restored across session switches. No-op when the chat ID is missing.
 */
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

/**
 * Remove the persisted active sidebar tab ID for a given chat. Called
 * when a chat is archived so a future unarchive starts fresh.
 */
export function clearPersistedSidebarTabId(chatID: string | undefined): void {
	if (!chatID) {
		return;
	}
	localStorage.removeItem(`${lastActiveSidebarTabStorageKeyPrefix}${chatID}`);
}
