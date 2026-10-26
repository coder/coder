// Async ask_user_question answers are ordinary user messages whose first
// line names the tool call they answer.
const asyncAnswerLead = "Answer to ask_user_question call";

const asyncAnswerPattern = new RegExp(`^${asyncAnswerLead} (\\S+):\n`);

/** Returns the prefix line, newline included, that starts an answer to the call. */
export const getAsyncAnswerPrefix = (toolCallId: string): string =>
	`${asyncAnswerLead} ${toolCallId}:\n`;

/** Reads the tool call ID from an answer's first line, if it has the prefix. */
export const getAsyncAnswerToolCallId = (
	responseText: string,
): string | undefined => asyncAnswerPattern.exec(responseText)?.[1];

/** Returns the text after the prefix line when it names this call. */
export const getAsyncAnswerText = (
	responseText: string | undefined,
	toolCallId: string,
): string | undefined => {
	const prefix = getAsyncAnswerPrefix(toolCallId);
	return responseText?.startsWith(prefix)
		? responseText.slice(prefix.length)
		: undefined;
};
