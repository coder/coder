// Async ask_user_question answers are ordinary user messages whose first
// line names the tool call they answer. The server trims each text part, so
// when a chip follows that line directly, the line ends its part.
const asyncAnswerLead = "Answer to ask_user_question call ";

const asyncAnswerPattern = new RegExp(`^${asyncAnswerLead}([^\\n]+):(?:\\n|$)`);

/** Returns the prefix line, newline included, that starts an answer to the call. */
export const getAsyncAnswerPrefix = (toolCallId: string): string =>
	`${asyncAnswerLead}${toolCallId}:\n`;

/** A prompt history entry's call line has no line break when a chip followed it. */
export const isAsyncAnswerPrompt = (text: string): boolean =>
	text.startsWith(asyncAnswerLead);

/** An answer split at its prefix line. */
export type AsyncAnswer = {
	toolCallId: string;
	/** The text after the prefix line. */
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
