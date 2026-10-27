// Async ask_user_question answers are ordinary user messages whose first
// line names the tool call they answer. The server trims each text part, so
// when a chip follows that line directly, the line ends its part.
const asyncAnswerPattern =
	/^Answer to ask_user_question call ([^\n]+):(?:\n|$)/;

/** Returns the prefix line, newline included, that starts an answer to the call. */
export const getAsyncAnswerPrefix = (toolCallId: string): string =>
	`Answer to ask_user_question call ${toolCallId}:\n`;

/** An answer message split into the call it names and its body. */
export type AsyncAnswer = {
	toolCallId: string;
	/** The answer without its prefix line, as users see it. */
	body: string;
};

/**
 * Returns undefined unless the text starts with the exact prefix line, which
 * may end the text without its line break.
 */
export const parseAsyncAnswer = (text: string): AsyncAnswer | undefined => {
	const match = asyncAnswerPattern.exec(text);
	if (!match) {
		return undefined;
	}
	return { toolCallId: match[1], body: text.slice(match[0].length) };
};
