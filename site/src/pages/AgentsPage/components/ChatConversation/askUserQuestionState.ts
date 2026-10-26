import type * as TypesGen from "#/api/typesGenerated";
import { parseAsyncAnswer } from "../ChatElements/tools/asyncAnswer";
import { isAsyncAskUserQuestionResult } from "../ChatElements/tools/utils";
import { getMessageText } from "./messageParsing";
import type { MergedTool, ParsedMessageEntry } from "./types";

type AskUserQuestionState = {
	/** The newest valid question. */
	latestToolId: string | undefined;
	/** Set only by delivered messages, never by queued ones. */
	isLatestAnswered: boolean;
	/** Answer message text by question call ID, from delivered and queued messages. */
	responseTextByToolId: ReadonlyMap<string, string>;
};

const getTextContent = (
	content: readonly TypesGen.ChatMessagePart[] | undefined,
): string | undefined => {
	const text = getMessageText(content);
	return text.length > 0 ? text : undefined;
};

const readAsyncAnswer = (
	content: readonly TypesGen.ChatMessagePart[] | undefined,
): { toolCallId: string; messageText: string } | undefined => {
	const messageText = getTextContent(content);
	if (messageText === undefined) {
		return undefined;
	}
	const answer = parseAsyncAnswer(messageText);
	return answer && { toolCallId: answer.toolCallId, messageText };
};

// A call that failed validation asked nothing, so it supersedes nothing.
const isValidAskUserQuestion = (tool: MergedTool): boolean =>
	tool.name === "ask_user_question" && !tool.isError;

/**
 * A blocking question is answered by the next user message; an async one
 * only by a message whose first line names its call.
 */
export const deriveAskUserQuestionState = (
	parsedMessages: readonly ParsedMessageEntry[],
	liveTools: readonly MergedTool[],
	queuedMessages: readonly TypesGen.ChatQueuedMessage[],
): AskUserQuestionState => {
	let latestToolId: string | undefined;
	let isLatestAnswered = false;
	let pendingBlockingToolId: string | undefined;
	const responseTextByToolId = new Map<string, string>();

	for (const { message, parsed } of parsedMessages) {
		for (const tool of parsed.tools) {
			if (!isValidAskUserQuestion(tool)) {
				continue;
			}
			latestToolId = tool.id;
			isLatestAnswered = false;
			pendingBlockingToolId = isAsyncAskUserQuestionResult(tool.result)
				? undefined
				: tool.id;
		}
		if (message.role !== "user") {
			continue;
		}

		const responseText = getTextContent(message.content);
		if (pendingBlockingToolId !== undefined) {
			isLatestAnswered = true;
			if (responseText !== undefined) {
				responseTextByToolId.set(pendingBlockingToolId, responseText);
			}
			pendingBlockingToolId = undefined;
		}
		const asyncAnswer = readAsyncAnswer(message.content);
		if (asyncAnswer) {
			responseTextByToolId.set(asyncAnswer.toolCallId, asyncAnswer.messageText);
			if (asyncAnswer.toolCallId === latestToolId) {
				isLatestAnswered = true;
			}
		}
	}

	// A question still in the stream is newer than every persisted one once
	// its result shows it is valid.
	const liveQuestion = liveTools.findLast(
		(tool) => isValidAskUserQuestion(tool) && tool.status === "completed",
	);
	if (liveQuestion) {
		latestToolId = liveQuestion.id;
		isLatestAnswered = false;
	}

	// A queued answer is not delivered yet, but it already answers its call,
	// so another tab or a reload does not offer the question again.
	for (const queuedMessage of queuedMessages) {
		const asyncAnswer = readAsyncAnswer(queuedMessage.content);
		if (asyncAnswer && !responseTextByToolId.has(asyncAnswer.toolCallId)) {
			responseTextByToolId.set(asyncAnswer.toolCallId, asyncAnswer.messageText);
		}
	}

	return { latestToolId, isLatestAnswered, responseTextByToolId };
};
