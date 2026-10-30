// Async ask_user_question answers are ordinary user messages whose first
// line names the tool call they answer. The server trims each text part, so
// when a chip follows that line directly, the line ends its part.
/** Starts the first line of every async answer, before the call ID. */
export const asyncAnswerLead = "Answer to ask_user_question call ";

const asyncAnswerPattern = new RegExp(`^${asyncAnswerLead}([^\\n]+):(?:\\n|$)`);

const asyncAnswerPromptPattern = new RegExp(`^${asyncAnswerLead}\\S+:`);

/**
 * Reports whether prompt history text is an async answer. The history joins
 * text parts, so a chip right after the prefix line removes its line break.
 */
export const isAsyncAnswerPrompt = (text: string): boolean =>
	asyncAnswerPromptPattern.test(text);

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

/**
 * Returns the answer to the async call without its prefix line, or the text
 * unchanged when asyncToolCallId is unset. Returns undefined when there is no
 * text or the text does not answer the async call.
 */
export const getAnswerBody = (
	text: string | undefined,
	asyncToolCallId: string | undefined,
): string | undefined => {
	if (text === undefined || asyncToolCallId === undefined) {
		return text;
	}
	const answer = parseAsyncAnswer(text);
	return answer?.toolCallId === asyncToolCallId ? answer.body : undefined;
};
