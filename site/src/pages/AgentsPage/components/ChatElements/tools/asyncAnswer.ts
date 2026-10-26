import { asRecord } from "../runtimeTypeUtils";
import { parseArgs } from "./utils";

// Async ask_user_question answers are ordinary user messages whose first
// line names the tool call they answer.
const asyncAnswerLead = "Answer to ask_user_question call";

const asyncAnswerPattern = new RegExp(`^${asyncAnswerLead} (\\S+):\n`);

/** Returns the prefix line, newline included, that starts an answer to the call. */
export const getAsyncAnswerPrefix = (toolCallId: string): string =>
	`${asyncAnswerLead} ${toolCallId}:\n`;

type AsyncAnswer = {
	toolCallId: string;
	/** The answer without its prefix line, as users see it. */
	body: string;
};

/** Splits an answer into the call it names and its body, if it starts with the prefix line. */
export const parseAsyncAnswer = (text: string): AsyncAnswer | undefined => {
	const match = asyncAnswerPattern.exec(text);
	if (!match) {
		return undefined;
	}
	return { toolCallId: match[1], body: text.slice(match[0].length) };
};

/** Returns the answer body when the text answers this call. */
export const getAsyncAnswerText = (
	responseText: string | undefined,
	toolCallId: string,
): string | undefined => {
	const answer =
		responseText === undefined ? undefined : parseAsyncAnswer(responseText);
	return answer?.toolCallId === toolCallId ? answer.body : undefined;
};

/** Returns the result object that carries the question list, in any shape the tool result arrives in. */
export const findAskUserQuestionResultRecord = (
	result: unknown,
): Record<string, unknown> | null => {
	const resultRecord = asRecord(result);
	for (const value of [
		result,
		resultRecord?.output,
		resultRecord?.content,
		resultRecord?.text,
	]) {
		const parsedValue = parseArgs(value);
		if (Array.isArray(parsedValue?.questions)) {
			return parsedValue;
		}
	}
	return null;
};

/** Reports whether the server marked the question as not ending the agent's turn. */
export const isAsyncAskUserQuestionResult = (result: unknown): boolean =>
	findAskUserQuestionResultRecord(result)?.async === true;
