import type { UseLocalStorageOptions } from "#/hooks/useLocalStorage";
import { booleanCodec } from "#/hooks/useLocalStorage";

/** Storage options for the chat full-width preference. Pass to `useLocalStorage`. */
export const chatFullWidthStorageOptions = {
	storageKey: "agents.chat-full-width",
	codec: booleanCodec(false),
} satisfies UseLocalStorageOptions<boolean>;

/**
 * Returns the Tailwind max-width class for the chat layout
 * based on whether full-width mode is enabled.
 */
export function chatWidthClass(fullWidth: boolean): string {
	return fullWidth ? "max-w-full" : "max-w-3xl";
}
