// Async ask_user_question answers are ordinary user messages whose first
// line names the tool call they answer.
const asyncAnswerPattern = /^Answer to ask_user_question call ([^\n]+):\n/;

/** Returns the prefix line, newline included, that starts an answer to the call. */
export const getAsyncAnswerPrefix = (toolCallId: string): string =>
	`Answer to ask_user_question call ${toolCallId}:\n`;

type AsyncAnswer = {
	toolCallId: string;
	/** The answer without its prefix line, as users see it. */
	body: string;
};

/** Returns undefined unless the text starts with the exact prefix line. */
export const parseAsyncAnswer = (text: string): AsyncAnswer | undefined => {
	const match = asyncAnswerPattern.exec(text);
	if (!match) {
		return undefined;
	}
	return { toolCallId: match[1], body: text.slice(match[0].length) };
};

/** Returns undefined unless the text is an answer to this call. */
export const getAsyncAnswerText = (
	responseText: string | undefined,
	toolCallId: string,
): string | undefined => {
	const answer =
		responseText === undefined ? undefined : parseAsyncAnswer(responseText);
	return answer?.toolCallId === toolCallId ? answer.body : undefined;
};
