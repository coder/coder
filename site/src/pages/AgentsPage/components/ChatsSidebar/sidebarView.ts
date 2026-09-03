type SidebarView =
	| { panel: "chats" }
	| { panel: "settings"; section: string | undefined };

/**
 * Derive the current sidebar view from the URL pathname.
 */
export function sidebarViewFromPath(pathname: string): SidebarView {
	const settingsMatch = pathname.match(/^\/agents\/settings(?:\/([^/]+))?/);
	if (settingsMatch) {
		return { panel: "settings", section: settingsMatch[1] };
	}
	return { panel: "chats" };
}

export function isSettingsView(
	view: SidebarView,
): view is Extract<SidebarView, { panel: "settings" }> {
	return view.panel === "settings";
}
