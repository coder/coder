import {
	isUserRightPanelTab,
	type RightPanelGroupTabId,
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

export const activeSubTabStorageKeyPrefix = "agents.right-panel-active-chips.";

export type ActiveSubTabIds = Partial<Record<RightPanelGroupTabId, string>>;

/**
 * The chip last selected inside each group tab. A missing or unreadable
 * entry means the group falls back to its first chip.
 */
export function getPersistedActiveSubTabIds(
	chatID: string | undefined,
): ActiveSubTabIds {
	if (!chatID) {
		return {};
	}

	const value = localStorage.getItem(
		`${activeSubTabStorageKeyPrefix}${chatID}`,
	);
	if (!value) {
		return {};
	}

	try {
		const parsed: unknown = JSON.parse(value);
		if (typeof parsed !== "object" || parsed === null) {
			return {};
		}
		const record = parsed as Record<string, unknown>;
		const result: ActiveSubTabIds = {};
		if (typeof record.terminal === "string") {
			result.terminal = record.terminal;
		}
		if (typeof record.workspace === "string") {
			result.workspace = record.workspace;
		}
		return result;
	} catch {
		return {};
	}
}

export function savePersistedActiveSubTabIds(
	chatID: string | undefined,
	ids: ActiveSubTabIds,
): void {
	if (!chatID) {
		return;
	}
	localStorage.setItem(
		`${activeSubTabStorageKeyPrefix}${chatID}`,
		JSON.stringify(ids),
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

/** Written by the previous strip, where Browser, Desktop, and Debug were opt-in. */
const legacyVisibleSingletonTabsStorageKeyPrefix =
	"agents.right-panel-singleton-tabs.";

export function clearPersistedRightPanelState(
	chatID: string | undefined,
): void {
	if (!chatID) {
		return;
	}
	localStorage.removeItem(`${rightPanelTabStorageKeyPrefix}${chatID}`);
	localStorage.removeItem(`${activeSubTabStorageKeyPrefix}${chatID}`);
	localStorage.removeItem(`${defaultTerminalHiddenStorageKeyPrefix}${chatID}`);
	localStorage.removeItem(
		`${legacyVisibleSingletonTabsStorageKeyPrefix}${chatID}`,
	);
}
