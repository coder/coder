/** Holds "false" after the user hides the overview card. */
export const overviewCardOpenStorageKey = "agents.overview-card-open";

/** Returns true when nothing is saved, so the card shows until the user first hides it. */
export function getPersistedOverviewCardOpen(): boolean {
	return localStorage.getItem(overviewCardOpenStorageKey) !== "false";
}

/** Stores the open state for every chat. */
export function savePersistedOverviewCardOpen(open: boolean): void {
	localStorage.setItem(overviewCardOpenStorageKey, String(open));
}
