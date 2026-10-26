// Async ask_user_question answers are ordinary user messages whose first
// line names the tool call they answer.
const asyncAnswerLead = "Answer to ask_user_question call";

const asyncAnswerPattern = new RegExp(`^${asyncAnswerLead} (\\S+):\n`);

/** Returns the first line of an answer to the given async question. */
export const getAsyncAnswerPrefix = (toolCallId: string): string =>
	`${asyncAnswerLead} ${toolCallId}:\n`;

/** Returns the tool call ID an async answer names, or undefined. */
export const getAsyncAnswerToolCallId = (
	responseText: string,
): string | undefined => asyncAnswerPattern.exec(responseText)?.[1];

/**
 * Returns the answer body when the text answers the given async question.
 * An unrelated later message does not answer it.
 */
export const getAsyncAnswerText = (
	responseText: string | undefined,
	toolCallId: string,
): string | undefined => {
	const prefix = getAsyncAnswerPrefix(toolCallId);
	return responseText?.startsWith(prefix)
		? responseText.slice(prefix.length)
		: undefined;
};
