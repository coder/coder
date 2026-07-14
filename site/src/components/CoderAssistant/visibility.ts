/**
 * Per-user visibility for the Coder Assistant. The assistant is
 * enabled per deployment via the coder-assistant experiment and shown
 * to everyone by default; individual users can hide it for themselves
 * in this browser. Stored in localStorage, so reads happen once at
 * mount and changes take effect after a page reload.
 */
export const CODER_ASSISTANT_HIDDEN_KEY = "coder_assistant_hidden";

export function isCoderAssistantHidden(): boolean {
	try {
		return localStorage.getItem(CODER_ASSISTANT_HIDDEN_KEY) === "true";
	} catch {
		return false;
	}
}

export function setCoderAssistantHidden(hidden: boolean): void {
	try {
		if (hidden) {
			localStorage.setItem(CODER_ASSISTANT_HIDDEN_KEY, "true");
		} else {
			localStorage.removeItem(CODER_ASSISTANT_HIDDEN_KEY);
		}
	} catch {
		// Storage may be unavailable in some contexts.
	}
}
