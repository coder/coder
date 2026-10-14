const projectsSectionCollapsedStorageKeyPrefix =
	"agents.sidebar.projects-collapsed.";

/**
 * Returns the localStorage key holding "true" while the user's sidebar
 * Projects section is collapsed. Keys are per user because the preference
 * outlives sign-out in a shared browser profile.
 */
export function projectsSectionCollapsedStorageKey(userId: string): string {
	return `${projectsSectionCollapsedStorageKeyPrefix}${userId}`;
}

/** Returns true when nothing is saved or storage is unavailable. */
export function getPersistedProjectsSectionExpanded(userId: string): boolean {
	try {
		return (
			localStorage.getItem(projectsSectionCollapsedStorageKey(userId)) !==
			"true"
		);
	} catch {
		return true;
	}
}

/**
 * Best effort: storage failures are ignored, so the state then lasts only
 * for the current page session.
 */
export function savePersistedProjectsSectionExpanded(
	userId: string,
	expanded: boolean,
): void {
	const key = projectsSectionCollapsedStorageKey(userId);
	try {
		if (expanded) {
			localStorage.removeItem(key);
		} else {
			localStorage.setItem(key, "true");
		}
	} catch {
		// Ignored, as documented above.
	}
}
