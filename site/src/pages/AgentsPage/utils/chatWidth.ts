/**
 * Tailwind max-width class for the chat layout based on whether
 * full-width mode is enabled. Shared so every chat surface (input,
 * transcript, skeletons) stays in lockstep.
 */
export function chatWidthClass(fullWidth: boolean): string {
	return fullWidth ? "max-w-full" : "max-w-3xl";
}
