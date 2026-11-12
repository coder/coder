import type * as TypesGen from "#/api/typesGenerated";
import {
	ASYNC_QUESTION_TOOL_NAME,
	getAsyncQuestionId,
} from "../ChatElements/tools/utils";
import type { MergedTool, ParsedMessageEntry } from "./types";

export type AsyncQuestionStatus = "open" | "pending" | "answered" | "closed";

/** One async question and where its answer is. */
export type AsyncQuestion = {
	questionId: string;
	/** The ask_user_question_async call, for its arguments. */
	tool: MergedTool;
	status: AsyncQuestionStatus;
	/** The answer's text while the status is pending or answered. */
	answerText: string | undefined;
};

export type AsyncQuestions = {
	byId: ReadonlyMap<string, AsyncQuestion>;
	/**
	 * The newest async question, when it is open or pending. The question
	 * panel shows it.
	 */
	panelQuestion: AsyncQuestion | undefined;
};

const collectQuestion = (
	byId: Map<string, AsyncQuestion>,
	tool: MergedTool,
): string | undefined => {
	if (tool.name !== ASYNC_QUESTION_TOOL_NAME || tool.isError) {
		return undefined;
	}

	const questionId = getAsyncQuestionId(tool.result);
	if (questionId === undefined) {
		return undefined;
	}

	if (!byId.has(questionId)) {
		byId.set(questionId, {
			questionId,
			tool,
			status: "open",
			answerText: undefined,
		});
	}

	return questionId;
};

const getQueuedAnswerId = (
	message: TypesGen.ChatQueuedMessage,
): string | undefined => {
	for (const part of message.content) {
		if (part.type === "question-answer" && part.question_id) {
			return part.question_id;
		}
	}

	return undefined;
};

const getQueuedAnswerText = (message: TypesGen.ChatQueuedMessage): string => {
	return message.content
		.filter((part) => part.type === "text")
		.map((part) => part.text)
		.join("");
};

const setQuestionStatus = (
	question: AsyncQuestion,
	newestQuestionId: string | undefined,
	deliveredAnswers: ReadonlyMap<string, string>,
	pendingAnswers: ReadonlyMap<string, string>,
): void => {
	if (deliveredAnswers.has(question.questionId)) {
		question.status = "answered";
		question.answerText = deliveredAnswers.get(question.questionId);
		return;
	}

	if (pendingAnswers.has(question.questionId)) {
		question.status = "pending";
		question.answerText = pendingAnswers.get(question.questionId);
		return;
	}

	if (question.questionId !== newestQuestionId) {
		question.status = "closed";
		return;
	}

	question.status = "open";
};

export const deriveAsyncQuestions = (
	parsedMessages: readonly ParsedMessageEntry[],
	liveTools: readonly MergedTool[],
	queuedMessages: readonly TypesGen.ChatQueuedMessage[],
): AsyncQuestions => {
	const byId = new Map<string, AsyncQuestion>();
	let newestQuestionId: string | undefined;
	for (const { parsed } of parsedMessages) {
		for (const tool of parsed.tools) {
			const questionId = collectQuestion(byId, tool);
			if (questionId !== undefined) {
				newestQuestionId = questionId;
			}
		}
	}
	for (const tool of liveTools) {
		const questionId = collectQuestion(byId, tool);
		if (questionId !== undefined) {
			newestQuestionId = questionId;
		}
	}

	const deliveredAnswers = new Map<string, string>();
	for (const { message, parsed } of parsedMessages) {
		if (message.role === "user" && parsed.questionAnswerId) {
			deliveredAnswers.set(parsed.questionAnswerId, parsed.markdown);
		}
	}

	const pendingAnswers = new Map<string, string>();
	for (const message of queuedMessages) {
		const questionId = getQueuedAnswerId(message);
		if (questionId !== undefined) {
			const answerText = getQueuedAnswerText(message);
			pendingAnswers.set(questionId, answerText);
		}
	}

	for (const question of byId.values()) {
		setQuestionStatus(
			question,
			newestQuestionId,
			deliveredAnswers,
			pendingAnswers,
		);
	}
	const newestQuestion = newestQuestionId
		? byId.get(newestQuestionId)
		: undefined;
	let panelQuestion: AsyncQuestion | undefined;
	if (
		newestQuestion?.status === "open" ||
		newestQuestion?.status === "pending"
	) {
		panelQuestion = newestQuestion;
	}
	return { byId, panelQuestion };
};
