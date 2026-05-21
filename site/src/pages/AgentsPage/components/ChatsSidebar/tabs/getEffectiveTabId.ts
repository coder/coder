/**
 * Resolves which sidebar tab should be active given the set of
 * available tab IDs, the currently stored selection, and whether
 * the desktop chat tab is available.
 */
export function getEffectiveTabId(
	tabIds: readonly string[],
	activeTabId: string | null,
	desktopChatId: string | undefined,
): string | null {
	const allIds = new Set(tabIds);
	if (desktopChatId) {
		allIds.add("desktop");
	}

	if (activeTabId !== null && allIds.has(activeTabId)) {
		return activeTabId;
	}

	return tabIds[0] ?? (desktopChatId ? "desktop" : null);
}
