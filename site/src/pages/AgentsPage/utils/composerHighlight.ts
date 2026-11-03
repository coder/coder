export const highlightComposerState = { highlightComposer: true } as const;

export const hasHighlightComposerState = (state: unknown): boolean =>
	typeof state === "object" &&
	state !== null &&
	"highlightComposer" in state &&
	state.highlightComposer === true;
