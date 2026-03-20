const STORAGE_KEY = "agents.collapsed-tool-calls";

export function getCollapsedToolCallsEnabled(): boolean {
	try {
		return localStorage.getItem(STORAGE_KEY) === "true";
	} catch {
		return false;
	}
}

export function setCollapsedToolCallsEnabled(enabled: boolean): void {
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
