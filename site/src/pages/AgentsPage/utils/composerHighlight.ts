/**
 * History state a "New chat" link sets to flash and focus the composer on
 * arrival. Without it, picking "New chat" while already on that composer
 * changes nothing on screen.
 */
export const highlightComposerState = { highlightComposer: true } as const;

/** Reports whether a history entry's state asks to highlight the composer. */
export const hasHighlightComposerState = (state: unknown): boolean =>
	typeof state === "object" &&
	state !== null &&
	"highlightComposer" in state &&
	state.highlightComposer === true;
