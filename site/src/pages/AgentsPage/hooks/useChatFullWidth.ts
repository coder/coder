import { useStorage } from "#/hooks/useStorage";
import { chatFullWidthStorage } from "#/utils/storage/keys";

/**
 * Returns the Tailwind max-width class for the chat layout
 * based on whether full-width mode is enabled.
 */
export function chatWidthClass(fullWidth: boolean): string {
	return fullWidth ? "max-w-full" : "max-w-3xl";
}

/**
 * Reactive hook for the chat full-width preference. All
 * consumers re-render when the value changes, in the same tab
 * and across tabs. No page reload required.
 */
export function useChatFullWidth(): [boolean, (v: boolean) => void] {
	const [enabled, setEnabled] = useStorage(chatFullWidthStorage);
	return [enabled, setEnabled];
}
