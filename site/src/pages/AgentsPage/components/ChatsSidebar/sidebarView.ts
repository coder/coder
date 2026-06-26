type SidebarView =
	| { panel: "chats" }
	| { panel: "settings"; section: string | undefined }
	| { panel: "settings-admin"; section: string | undefined }
	| { panel: "analytics" };

const ADMIN_SETTINGS_SECTIONS = new Set([
	"agents",
	"mcp-servers",
	"spend",
	"experiments",
]);

/**
 * Derive the current sidebar view from the URL pathname.
 */
export function sidebarViewFromPath(pathname: string): SidebarView {
	if (pathname.startsWith("/agents/analytics")) {
		return { panel: "analytics" };
	}
	const settingsMatch = pathname.match(/^\/agents\/settings(?:\/([^/]+))?/);
	if (settingsMatch) {
		const section = settingsMatch[1];
		if (section === "admin") {
			return { panel: "settings-admin", section: undefined };
		}
		return {
			panel: ADMIN_SETTINGS_SECTIONS.has(section ?? "")
				? "settings-admin"
				: "settings",
			section,
		};
	}
	return { panel: "chats" };
}

export function isSettingsView(
	view: SidebarView,
): view is Extract<SidebarView, { panel: "settings" | "settings-admin" }> {
	return view.panel === "settings" || view.panel === "settings-admin";
}
