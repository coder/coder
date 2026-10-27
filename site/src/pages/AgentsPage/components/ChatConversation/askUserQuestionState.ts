import type * as TypesGen from "#/api/typesGenerated";
import { isAfter } from "#/utils/time";
import { isAsyncAskUserQuestionResult } from "../ChatElements/tools/utils";
import { getMessageText, parseAsyncAnswerContent } from "./messageParsing";
import type { MergedTool, ParsedMessageEntry } from "./types";

/** Question progress derived from the transcript, the live stream and the queue. */
export type AskUserQuestionState = {
	/** Compare by identity: a provider can reuse an older call's ID. */
	latestTool: MergedTool | undefined;
	/** Set only by delivered messages, never by queued ones. */
	isLatestAnswered: boolean;
	/** Queued async answers are included; async answers keep their prefix line. */
	responseTextByTool: ReadonlyMap<MergedTool, string>;
	/**
	 * The latest question while it is async and has no delivered answer. A
	 * queued answer leaves it open because the queue can drop the answer
	 * before delivery.
	 */
	openAsyncQuestion: MergedTool | undefined;
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
	let pendingBlockingTool: MergedTool | undefined;
	const responseTextByTool = new Map<MergedTool, string>();
	// Providers can reuse a call ID in a later turn, so an answer belongs to
	// the newest call before it that has its ID.
	const latestToolById = new Map<string, MergedTool>();

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
			latestToolById.set(tool.id, tool);
			pendingBlockingTool = isAsyncAskUserQuestionResult(tool.result)
				? undefined
				: tool;
		}
		if (message.role !== "user") {
			continue;
		}

		const asyncAnswer = parseAsyncAnswerContent(message.content);
		if (asyncAnswer) {
			const answeredTool = latestToolById.get(asyncAnswer.toolCallId);
			if (answeredTool) {
				responseTextByTool.set(answeredTool, asyncAnswer.text);
			}
			if (asyncAnswer.toolCallId === latestTool?.id) {
				isLatestAnswered = true;
				pendingBlockingTool = undefined;
			}
		} else if (pendingBlockingTool !== undefined) {
			isLatestAnswered = true;
			const responseText = getMessageText(message.content);
			if (responseText.length > 0) {
				responseTextByTool.set(pendingBlockingTool, responseText);
			}
			pendingBlockingTool = undefined;
		}
	}

	// A question still in the stream is newer than every persisted one.
	const liveQuestion = liveTools.findLast(isValidAskUserQuestion);
	if (liveQuestion) {
		latestTool = liveQuestion;
		isLatestAnswered = false;
		latestToolById.set(liveQuestion.id, liveQuestion);
	}

	const openAsyncQuestion =
		latestTool &&
		isAsyncAskUserQuestionResult(latestTool.result) &&
		!responseTextByTool.has(latestTool)
			? latestTool
			: undefined;

	// A queued answer only records its text, so the card shows it and the
	// page hides the dock. openAsyncQuestion is set before this loop, so the
	// dock comes back if the queue drops the answer.
	for (const queuedMessage of queuedMessages) {
		const asyncAnswer = parseAsyncAnswerContent(queuedMessage.content);
		const answeredTool =
			asyncAnswer && latestToolById.get(asyncAnswer.toolCallId);
		if (
			asyncAnswer &&
			answeredTool &&
			!responseTextByTool.has(answeredTool) &&
			!isQueuedBeforeCall(queuedMessage, answeredTool.startedAt)
		) {
			responseTextByTool.set(answeredTool, asyncAnswer.text);
		}
	}

	return {
		latestTool,
		isLatestAnswered,
		responseTextByTool,
		openAsyncQuestion,
	};
};
