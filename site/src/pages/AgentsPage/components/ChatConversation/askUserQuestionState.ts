import type * as TypesGen from "#/api/typesGenerated";
import { getAsyncAnswerPrefix } from "../ChatElements/tools/asyncAnswer";
import { isAsyncAskUserQuestionResult } from "../ChatElements/tools/utils";
import { getMessageText, parseAsyncAnswerContent } from "./messageParsing";
import type { MergedTool, ParsedMessageEntry } from "./types";

type AskUserQuestionState = {
	latestToolId: string | undefined;
	/** Set only by delivered messages, never by queued ones. */
	isLatestAnswered: boolean;
	/** Whole answer messages, prefix line included, from delivered and queued messages. */
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
	const answer = parseAsyncAnswerContent(content);
	return (
		answer && {
			toolCallId: answer.toolCallId,
			messageText: getAsyncAnswerPrefix(answer.toolCallId) + answer.body,
		}
	);
};

// A call that failed validation asked nothing, so it supersedes nothing.
const isValidAskUserQuestion = (tool: MergedTool): boolean =>
	tool.name === "ask_user_question" && !tool.isError;

/**
 * A blocking question is answered by the next user message that does not
 * answer another call; an async one only by a message whose first line
 * names its call.
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

		const asyncAnswer = readAsyncAnswer(message.content);
		if (asyncAnswer) {
			responseTextByToolId.set(asyncAnswer.toolCallId, asyncAnswer.messageText);
			if (asyncAnswer.toolCallId === latestToolId) {
				isLatestAnswered = true;
				pendingBlockingToolId = undefined;
			}
		} else if (pendingBlockingToolId !== undefined) {
			isLatestAnswered = true;
			const responseText = getTextContent(message.content);
			if (responseText !== undefined) {
				responseTextByToolId.set(pendingBlockingToolId, responseText);
			}
			pendingBlockingToolId = undefined;
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
