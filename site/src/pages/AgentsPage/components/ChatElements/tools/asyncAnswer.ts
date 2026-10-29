// Async ask_user_question answers are ordinary user messages whose first
// line names the tool call they answer. The server trims each text part, so
// when a chip follows that line directly, the line ends its part.
/** Starts the first line of every async answer, before the call ID. */
export const asyncAnswerLead = "Answer to ask_user_question call ";

const asyncAnswerPattern = new RegExp(`^${asyncAnswerLead}([^\\n]+):(?:\\n|$)`);

/** Returns the prefix line, newline included, that starts an answer to the call. */
export const getAsyncAnswerPrefix = (toolCallId: string): string =>
	`${asyncAnswerLead}${toolCallId}:\n`;

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

/** Returns the text after the prefix line when the text answers the call. */
export const getAsyncAnswerBody = (
	text: string | undefined,
	toolCallId: string,
): string | undefined => {
	if (text === undefined) {
		return undefined;
	}
	const answer = parseAsyncAnswer(text);
	return answer?.toolCallId === toolCallId ? answer.body : undefined;
};
