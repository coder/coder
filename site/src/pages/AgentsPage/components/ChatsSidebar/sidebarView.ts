type SidebarView =
	| { panel: "chats"; projectId?: string }
	| { panel: "settings"; section: string | undefined };

/**
 * Derive the current sidebar view from the URL pathname.
 */
export function sidebarViewFromPath(pathname: string): SidebarView {
	const settingsMatch = pathname.match(/^\/agents\/settings(?:\/([^/]+))?/);
	if (settingsMatch) {
		return { panel: "settings", section: settingsMatch[1] };
	}
	const projectMatch = pathname.match(/^\/agents\/projects\/([^/]+)/);
	if (projectMatch) {
		return { panel: "chats", projectId: projectMatch[1] };
	}
	return { panel: "chats" };
}

export function isSettingsView(
	view: SidebarView,
): view is Extract<SidebarView, { panel: "settings" }> {
	return view.panel === "settings";
}
