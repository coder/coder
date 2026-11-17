import { booleanCodec, useLocalStorage } from "#/hooks/useLocalStorage";

const KEY = "agents.chat-full-width";

/**
 * Returns the Tailwind max-width class for the chat layout
 * based on whether full-width mode is enabled.
 */
export function chatWidthClass(fullWidth: boolean): string {
	return fullWidth ? "max-w-full" : "max-w-3xl";
}

/**
 * Reactive hook for the chat full-width preference. All
 * consumers re-render when the value changes — no page reload
 * required.
 */
export function useChatFullWidth(): [boolean, (v: boolean) => void] {
	return useLocalStorage(KEY, booleanCodec(false));
}
