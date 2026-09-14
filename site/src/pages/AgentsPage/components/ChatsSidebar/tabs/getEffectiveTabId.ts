/**
 * Resolves which sidebar tab should be active given the set of
 * available tab IDs and the currently stored selection.
 */
export function getEffectiveTabId(
	tabIds: readonly string[],
	activeTabId: string | null,
): string | null {
	if (activeTabId !== null && tabIds.includes(activeTabId)) {
		return activeTabId;
	}

	return tabIds[0] ?? null;
}
