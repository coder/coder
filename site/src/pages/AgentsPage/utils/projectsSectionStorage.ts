/** localStorage key holding "true" while the sidebar Projects section is collapsed. */
export const projectsSectionCollapsedStorageKey =
	"agents.sidebar.projects-collapsed";

/** Reads whether the sidebar Projects section is expanded, defaulting to true. */
export function getPersistedProjectsSectionExpanded(): boolean {
	try {
		return localStorage.getItem(projectsSectionCollapsedStorageKey) !== "true";
	} catch {
		return true;
	}
}

/** Saves whether the sidebar Projects section is expanded. */
export function savePersistedProjectsSectionExpanded(expanded: boolean): void {
	try {
		if (expanded) {
			localStorage.removeItem(projectsSectionCollapsedStorageKey);
		} else {
			localStorage.setItem(projectsSectionCollapsedStorageKey, "true");
		}
	} catch {
		// Ignore storage failures because toggling still works for this session.
	}
}
