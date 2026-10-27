import type * as TypesGen from "#/api/typesGenerated";
import { isAfter } from "#/utils/time";
import { isAsyncAskUserQuestionResult } from "../ChatElements/tools/utils";
import { getMessageText, parseAsyncAnswerContent } from "./messageParsing";
import type { MergedTool, ParsedMessageEntry } from "./types";

type AskUserQuestionState = {
	/** Compare by identity: a provider can reuse an older call's ID. */
	latestTool: MergedTool | undefined;
	/** Set only by delivered messages, never by queued ones. */
	isLatestAnswered: boolean;
	/** Whole answer messages, prefix line included, from delivered and queued messages. */
	responseTextByToolId: ReadonlyMap<string, string>;
};

// A call that is still running or failed validation supersedes nothing.
const isValidAskUserQuestion = (tool: MergedTool): boolean =>
	tool.name === "ask_user_question" && tool.status === "completed";

// Providers can reuse a call ID, so an answer queued before the newest call
// with its ID answers an older call.
const isQueuedBeforeCall = (
	queuedMessage: TypesGen.ChatQueuedMessage,
	callStartedAt: string | undefined,
): boolean =>
	callStartedAt !== undefined &&
	isAfter(callStartedAt, queuedMessage.created_at);

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
	let latestTool: MergedTool | undefined;
	let isLatestAnswered = false;
	let pendingBlockingToolId: string | undefined;
	const responseTextByToolId = new Map<string, string>();
	const startedAtByToolId = new Map<string, string | undefined>();

	for (const { message, parsed } of parsedMessages) {
		// The calls before a tool message already carry its results.
		if (message.role === "tool") {
			continue;
		}
		for (const tool of parsed.tools) {
			if (!isValidAskUserQuestion(tool)) {
				continue;
			}
			latestTool = tool;
			isLatestAnswered = false;
			// Providers can reuse a call ID in a later turn, so only the answers
			// that follow this call count for it.
			responseTextByToolId.delete(tool.id);
			startedAtByToolId.set(tool.id, tool.startedAt);
			pendingBlockingToolId = isAsyncAskUserQuestionResult(tool.result)
				? undefined
				: tool.id;
		}
		if (message.role !== "user") {
			continue;
		}

		const asyncAnswer = parseAsyncAnswerContent(message.content);
		if (asyncAnswer) {
			responseTextByToolId.set(asyncAnswer.toolCallId, asyncAnswer.text);
			if (asyncAnswer.toolCallId === latestTool?.id) {
				isLatestAnswered = true;
				pendingBlockingToolId = undefined;
			}
		} else if (pendingBlockingToolId !== undefined) {
			isLatestAnswered = true;
			const responseText = getMessageText(message.content);
			if (responseText.length > 0) {
				responseTextByToolId.set(pendingBlockingToolId, responseText);
			}
			pendingBlockingToolId = undefined;
		}
	}

	// A question still in the stream is newer than every persisted one.
	const liveQuestion = liveTools.findLast(isValidAskUserQuestion);
	if (liveQuestion) {
		latestTool = liveQuestion;
		isLatestAnswered = false;
		responseTextByToolId.delete(liveQuestion.id);
		startedAtByToolId.set(liveQuestion.id, liveQuestion.startedAt);
	}

	// A queued answer is not delivered yet, but it already answers its call,
	// so another tab or a reload does not offer the question again.
	for (const queuedMessage of queuedMessages) {
		const asyncAnswer = parseAsyncAnswerContent(queuedMessage.content);
		if (
			asyncAnswer &&
			!responseTextByToolId.has(asyncAnswer.toolCallId) &&
			!isQueuedBeforeCall(
				queuedMessage,
				startedAtByToolId.get(asyncAnswer.toolCallId),
			)
		) {
			responseTextByToolId.set(asyncAnswer.toolCallId, asyncAnswer.text);
		}
	}

	return { latestTool, isLatestAnswered, responseTextByToolId };
};
