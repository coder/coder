/**
 * Resolves which sidebar tab should be active given the set of
 * available tab IDs, the currently stored selection, and whether
 * the desktop chat tab is available.
 *
 * Precedence:
 * 1. `activeTabId` when it matches a known tab.
 * 2. The first entry in `tabIds` (ordered array, not a Set).
 * 3. `"desktop"` when `desktopChatId` is truthy.
 * 4. `null` (no valid tab available).
 *
 * AgentChatPageView owns this resolution so the parent-side gating
 * (e.g. `TerminalPanel.isVisible`, `DebugPanel.isVisible`) and the
 * child SidebarTabView's visual highlight always agree. The child
 * receives the resolved value via the `effectiveTabId` prop.
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
