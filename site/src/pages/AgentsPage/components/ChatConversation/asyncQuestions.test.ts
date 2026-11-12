import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import {
	type AsyncQuestionStatus,
	deriveAsyncQuestions,
} from "./asyncQuestions";
import { parseMessageContent } from "./messageParsing";
import type { MergedTool, ParsedMessageEntry } from "./types";

const makeTool = (
	questionId: string,
	overrides: Partial<MergedTool> = {},
): MergedTool => ({
	id: `call-${questionId}`,
	name: "ask_user_question_async",
	result: { question_id: questionId, status: "Continue working." },
	status: "completed",
	isError: false,
	...overrides,
});

const toolMessage = (...tools: MergedTool[]): ParsedMessageEntry => ({
	message: { ...MockChatMessage, role: "assistant" },
	parsed: { ...parseMessageContent([]), tools },
});

const userMessage = (text: string, questionId?: string): ParsedMessageEntry => {
	const content: TypesGen.ChatMessagePart[] = [{ type: "text", text }];
	if (questionId !== undefined) {
		content.unshift({ type: "question-answer", question_id: questionId });
	}

	return {
		message: { ...MockChatMessage, role: "user", content },
		parsed: parseMessageContent(content),
	};
};

const queuedMessage = (
	questionId: string | undefined,
): TypesGen.ChatQueuedMessage => ({
	id: 1,
	chat_id: MockChatMessage.chat_id,
	created_at: MockChatMessage.created_at,
	busy_behavior: "steer",
	content: [
		{ type: "question-answer", question_id: questionId },
		{ type: "text", text: "Queued " },
		{ type: "text", text: "answer" },
	],
});

describe("deriveAsyncQuestions", () => {
	it("returns no questions or panel question without async questions", () => {
		const result = deriveAsyncQuestions([], [], []);

		expect(result.byId.size).toBe(0);
		expect(result.panelQuestion).toBeUndefined();
	});

	it("opens one async question in the question panel", () => {
		const result = deriveAsyncQuestions([toolMessage(makeTool("one"))], [], []);

		expect(result.byId.get("one")).toMatchObject({
			questionId: "one",
			status: "open",
			answerText: undefined,
		});
		expect(result.panelQuestion?.questionId).toBe("one");
	});

	it("does not collect a blocking question", () => {
		const tool = makeTool("one", { name: "ask_user_question" });
		const result = deriveAsyncQuestions([toolMessage(tool)], [], []);

		expect(result.byId.size).toBe(0);
		expect(result.panelQuestion).toBeUndefined();
	});

	it("marks an answer in the timeline as answered", () => {
		const result = deriveAsyncQuestions(
			[toolMessage(makeTool("one")), userMessage("Redis", "one")],
			[],
			[],
		);

		expect(result.byId.get("one")).toMatchObject({
			status: "answered",
			answerText: "Redis",
		});
		expect(result.panelQuestion).toBeUndefined();
	});

	it("leaves a question open after an ordinary user message", () => {
		const result = deriveAsyncQuestions(
			[toolMessage(makeTool("one")), userMessage("Keep working")],
			[],
			[],
		);

		expect(result.byId.get("one")?.status).toBe("open");
	});

	it("leaves a question open after an answer to another question", () => {
		const result = deriveAsyncQuestions(
			[toolMessage(makeTool("one")), userMessage("Redis", "other")],
			[],
			[],
		);

		expect(result.byId.get("one")?.status).toBe("open");
	});

	it("keeps a queued answer pending in the question panel", () => {
		const result = deriveAsyncQuestions(
			[toolMessage(makeTool("one"))],
			[],
			[queuedMessage(undefined), queuedMessage(""), queuedMessage("one")],
		);

		expect(result.byId.get("one")).toMatchObject({
			status: "pending",
			answerText: "Queued answer",
		});
		expect(result.panelQuestion?.questionId).toBe("one");
	});

	it("prefers a delivered answer over a queued answer with the same ID", () => {
		const result = deriveAsyncQuestions(
			[toolMessage(makeTool("one")), userMessage("Delivered answer", "one")],
			[],
			[queuedMessage("one")],
		);

		expect(result.byId.get("one")).toMatchObject({
			status: "answered",
			answerText: "Delivered answer",
		});
		expect(result.panelQuestion).toBeUndefined();
	});

	it("closes the older async question when another arrives", () => {
		const result = deriveAsyncQuestions(
			[toolMessage(makeTool("one"), makeTool("two"))],
			[],
			[],
		);

		expect(result.byId.get("one")?.status).toBe("closed");
		expect(result.byId.get("two")?.status).toBe("open");
		expect(result.panelQuestion?.questionId).toBe("two");
	});

	it("does not close an async question for a newer blocking question", () => {
		const blockingTool = makeTool("two", { name: "ask_user_question" });
		const result = deriveAsyncQuestions(
			[toolMessage(makeTool("one"), blockingTool)],
			[],
			[],
		);

		expect(result.byId.get("one")?.status).toBe("open");
		expect(result.panelQuestion?.questionId).toBe("one");
	});

	it.each([
		{ status: "running", result: undefined, isError: false },
		{ status: "error", isError: true },
	] satisfies Partial<MergedTool>[])(
		"does not close an async question for a newer $status call",
		(overrides) => {
			const result = deriveAsyncQuestions(
				[toolMessage(makeTool("one"), makeTool("two", overrides))],
				[],
				[],
			);

			expect(result.byId.size).toBe(1);
			expect(result.byId.get("one")?.status).toBe("open");
			expect(result.panelQuestion?.questionId).toBe("one");
		},
	);

	it.each([
		{
			status: "answered",
			answerMessages: [userMessage("Redis", "one")],
			queuedMessages: [],
		},
		{
			status: "pending",
			answerMessages: [],
			queuedMessages: [queuedMessage("one")],
		},
	] satisfies {
		status: AsyncQuestionStatus;
		answerMessages: ParsedMessageEntry[];
		queuedMessages: TypesGen.ChatQueuedMessage[];
	}[])(
		"marks an older question $status without replacing the newer panel question",
		({ status, answerMessages, queuedMessages }) => {
			const messages = [
				toolMessage(makeTool("one"), makeTool("two")),
				...answerMessages,
			];

			const result = deriveAsyncQuestions(messages, [], queuedMessages);

			expect(result.byId.get("one")?.status).toBe(status);
			expect(result.panelQuestion?.questionId).toBe("two");
		},
	);

	it("keeps the first tool when a question also appears in live tools", () => {
		const tool = makeTool("one");
		const liveTool = makeTool("one", { id: "repeated-call" });
		const result = deriveAsyncQuestions([toolMessage(tool)], [liveTool], []);

		expect(result.byId.size).toBe(1);
		expect(result.byId.get("one")).toMatchObject({ tool, status: "open" });
	});

	it("opens a question that appears only in live tools", () => {
		const result = deriveAsyncQuestions([], [makeTool("one")], []);

		expect(result.byId.get("one")?.status).toBe("open");
		expect(result.panelQuestion?.questionId).toBe("one");
	});
});
