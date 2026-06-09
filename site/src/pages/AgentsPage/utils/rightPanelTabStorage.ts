import { isUserRightPanelTab, type UserRightPanelTab } from "./rightPanelTabs";

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

const defaultTerminalHiddenStorageKeyPrefix = "agents.default-terminal-hidden.";

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
	localStorage.removeItem(`${defaultTerminalHiddenStorageKeyPrefix}${chatID}`);
}
