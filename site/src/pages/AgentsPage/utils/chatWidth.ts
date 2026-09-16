/** Returns the Tailwind max-width class for the selected chat layout. */
export function chatWidthClass(fullWidth: boolean): string {
	return fullWidth ? "max-w-full" : "max-w-3xl";
}
