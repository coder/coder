import {
	chatDefaultTerminalHiddenStorage,
	chatRightPanelTabsStorage,
} from "#/utils/storage/keys";
import { isUserRightPanelTab, type UserRightPanelTab } from "./rightPanelTabs";

export function getPersistedRightPanelTabs(
	chatID: string | undefined,
): UserRightPanelTab[] {
	if (!chatID) {
		return [];
	}
	// Stored values are raw strings; narrow so stale tab IDs from
	// older builds are dropped on read.
	return chatRightPanelTabsStorage
		.forId(chatID)
		.get()
		.filter(isUserRightPanelTab);
}

export function savePersistedRightPanelTabs(
	chatID: string | undefined,
	tabs: readonly UserRightPanelTab[],
): void {
	if (!chatID) {
		return;
	}
	chatRightPanelTabsStorage.forId(chatID).set(tabs);
}

export function getPersistedDefaultTerminalHidden(
	chatID: string | undefined,
): boolean {
	if (!chatID) {
		return false;
	}
	return chatDefaultTerminalHiddenStorage.forId(chatID).get();
}

export function savePersistedDefaultTerminalHidden(
	chatID: string | undefined,
	hidden: boolean,
): void {
	if (!chatID) {
		return;
	}
	const handle = chatDefaultTerminalHiddenStorage.forId(chatID);
	if (hidden) {
		handle.set(true);
	} else {
		handle.remove();
	}
}
