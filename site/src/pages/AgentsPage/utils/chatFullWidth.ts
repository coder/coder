import { booleanCodec, localStorageOptions } from "#/hooks/useLocalStorage";

/** Storage options for the chat full-width preference. */
export const chatFullWidthStorageOptions = localStorageOptions({
	storageKey: "agents.chat-full-width",
	codec: booleanCodec(false),
});

/**
 * Returns the Tailwind max-width class for the chat layout
 * based on whether full-width mode is enabled.
 */
export function chatWidthClass(fullWidth: boolean): string {
	return fullWidth ? "max-w-full" : "max-w-3xl";
}
