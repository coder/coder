/**
 * Tailwind max-width class for the chat layout based on whether
 * full-width mode is enabled.
 */
export function chatWidthClass(fullWidth: boolean): string {
	return fullWidth ? "max-w-full" : "max-w-3xl";
}
