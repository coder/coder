const STORAGE_KEY = "agents.working-blocks";

export function getWorkingBlocksEnabled(): boolean {
	try {
		return localStorage.getItem(STORAGE_KEY) === "true";
	} catch {
		return false;
	}
}

export function setWorkingBlocksEnabled(enabled: boolean): void {
	try {
		if (enabled) {
			localStorage.setItem(STORAGE_KEY, "true");
		} else {
			localStorage.removeItem(STORAGE_KEY);
		}
	} catch {
		// Silently ignore storage errors.
	}
}
