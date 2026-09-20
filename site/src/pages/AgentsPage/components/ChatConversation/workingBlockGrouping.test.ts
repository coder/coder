import type * as TypesGen from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { buildDisplayMessages } from "./messageHelpers";
import { parseMessagesWithMergedTools } from "./messageParsing";
import {
	workingFixtureTime as at,
	buildStreamRenderState,
	WORKING_FIXTURE_START,
} from "./storyFixtures";
import { assignTimelineRows, type TimelineRow } from "./timelineRows";
import {
	formatWorkingDuration,
	type GroupWorkingBlocksOptions,
	groupWorkingBlocks,
	type WorkingBlock,
} from "./workingBlockGrouping";

let nextId = 1;
const message = (
	role: TypesGen.ChatMessageRole,
	content: TypesGen.ChatMessagePart[],
	createdAt = at(0),
): TypesGen.ChatMessage => ({
	...MockChatMessage,
	id: nextId++,
	role,
	created_at: createdAt,
	content,
});

const user = (text: string) => message("user", [{ type: "text", text }]);
const text = (value: string): TypesGen.ChatMessagePart => ({
	type: "text",
	text: value,
});
const reasoning = (
	value: string,
	createdAt?: string,
	completedAt?: string,
): TypesGen.ChatMessagePart => ({
	type: "reasoning",
	text: value,
	created_at: createdAt,
	completed_at: completedAt,
});
const call = (
	id: string,
	createdAt?: string,
	name = "execute",
): TypesGen.ChatMessagePart => ({
	type: "tool-call",
	tool_call_id: id,
	tool_name: name,
	args: { command: `echo ${id}` },
	created_at: createdAt,
});
const result = (
	id: string,
	createdAt?: string,
	options: { isError?: boolean; name?: string } = {},
): TypesGen.ChatMessagePart => ({
	type: "tool-result",
	tool_call_id: id,
	tool_name: options.name ?? "execute",
	result: { output: id },
	is_error: options.isError,
	created_at: createdAt,
});

/** One persisted step: an assistant tool call followed by its hidden result. */
const step = (
	id: string,
	callAt?: number,
	resultAt?: number,
	options: { isError?: boolean; leading?: TypesGen.ChatMessagePart[] } = {},
): TypesGen.ChatMessage[] => [
	message(
		"assistant",
		[
			...(options.leading ?? []),
			call(id, callAt === undefined ? undefined : at(callAt)),
		],
		callAt === undefined ? at(0) : at(callAt),
	),
	message(
		"tool",
		[
			result(id, resultAt === undefined ? undefined : at(resultAt), {
				isError: options.isError,
			}),
		],
		resultAt === undefined ? at(0) : at(resultAt),
	),
];

const defaultOptions: GroupWorkingBlocksOptions = {
	hasMoreMessages: false,
	isTurnActive: false,
	isLiveRowCollapsible: false,
	liveBlocks: [],
	liveTools: [],
};

const group = (
	messages: readonly TypesGen.ChatMessage[],
	options: Partial<GroupWorkingBlocksOptions> = {},
): { rows: readonly TimelineRow[]; blocks: WorkingBlock[] } => {
	const entries = parseMessagesWithMergedTools(messages);
	const hasLive = options.isTurnActive ?? false;
	const rows = assignTimelineRows(buildDisplayMessages(entries), hasLive);
	return {
		rows,
		blocks: groupWorkingBlocks(rows, entries, {
			...defaultOptions,
			...options,
		}),
	};
};

const rowIds = (rows: readonly TimelineRow[], indices: readonly number[]) =>
	indices.map((i) => {
		const row = rows[i];
		return row.type === "live" ? "live" : row.entry.message.id;
	});

beforeEach(() => {
	nextId = 1;
});

describe("groupWorkingBlocks", () => {
	it("folds a turn's tool steps and leaves the answer and prompt visible", () => {
		const prompt = user("Inspect");
		const steps = [...step("a", 1, 4), ...step("b", 5, 13)];
		const answer = message("assistant", [text("Done.")], at(14));
		const { rows, blocks } = group([prompt, ...steps, answer]);

		expect(blocks).toHaveLength(1);
		const [block] = blocks;
		expect(rowIds(rows, block.rowIndices)).toEqual([steps[0].id, steps[2].id]);
		expect(block).toMatchObject({
			stepCount: 2,
			failedCount: 0,
			isLive: false,
			isPartial: false,
			startedAt: WORKING_FIXTURE_START + 1000,
			endedAt: WORKING_FIXTURE_START + 13000,
			key: `working:through:message:${steps[2].id}`,
			liveKey: `working:live:message:${prompt.id}:0`,
		});
	});

	it("treats reasoning and narration before a tool call as part of the step", () => {
		const prompt = user("Go");
		const steps = [
			...step("a", 1, 2, {
				leading: [reasoning("Plan", at(0.5), at(1))],
			}),
			...step("b", 3, 4, { leading: [text("Let me check the tests.")] }),
		];
		const answer = message("assistant", [reasoning("Wrap up"), text("Done.")]);
		const { rows, blocks } = group([prompt, ...steps, answer]);

		expect(blocks).toHaveLength(1);
		expect(rowIds(rows, blocks[0].rowIndices)).toEqual([
			steps[0].id,
			steps[2].id,
		]);
		// Reasoning start counts toward the wall-clock span.
		expect(blocks[0].startedAt).toBe(WORKING_FIXTURE_START + 500);
	});

	it("leaves a standalone reasoning-only row unfolded", () => {
		const prompt = user("Go");
		const thinking = message("assistant", [reasoning("Just thinking", at(1))]);
		const answer = message("assistant", [text("Done.")], at(2));
		const { blocks } = group([prompt, thinking, answer]);
		expect(blocks).toEqual([]);
	});

	it("folds a reasoning-only row that sits between tool steps", () => {
		const prompt = user("Go");
		const first = step("a", 1, 2);
		const thinking = message("assistant", [reasoning("Hmm", at(3), at(4))]);
		const second = step("b", 5, 6);
		const { rows, blocks } = group([prompt, ...first, thinking, ...second]);

		expect(blocks).toHaveLength(1);
		expect(rowIds(rows, blocks[0].rowIndices)).toEqual([
			first[0].id,
			thinking.id,
			second[0].id,
		]);
		expect(blocks[0].stepCount).toBe(2);
	});

	it("splits a turn at an interleaved answer row", () => {
		const prompt = user("Go");
		const first = step("a", 1, 2);
		const interlude = message("assistant", [text("Halfway there.")], at(3));
		const second = step("b", 4, 5);
		const { rows, blocks } = group([prompt, ...first, interlude, ...second]);

		expect(blocks).toHaveLength(2);
		expect(rowIds(rows, blocks[0].rowIndices)).toEqual([first[0].id]);
		expect(rowIds(rows, blocks[1].rowIndices)).toEqual([second[0].id]);
		expect(blocks[0].liveKey).toBe(`working:live:message:${prompt.id}:0`);
		expect(blocks[1].liveKey).toBe(`working:live:message:${prompt.id}:1`);
	});

	it.each(["ask_user_question", "propose_plan", "chat_summarized"])(
		"never folds a row containing %s",
		(name) => {
			const prompt = user("Go");
			const before = step("a", 1, 2);
			const special = [
				message("assistant", [call("q", at(3), name)]),
				message("tool", [result("q", at(4), { name })]),
			];
			const after = step("b", 5, 6);
			const { rows, blocks } = group([prompt, ...before, ...special, ...after]);

			expect(blocks).toHaveLength(2);
			expect(blocks.flatMap((b) => rowIds(rows, b.rowIndices))).toEqual([
				before[0].id,
				after[0].id,
			]);
		},
	);

	it("keeps failed steps inside the block and counts them", () => {
		const prompt = user("Go");
		const steps = [...step("a", 1, 2, { isError: true }), ...step("b", 3, 4)];
		const { blocks } = group([prompt, ...steps]);

		expect(blocks).toHaveLength(1);
		expect(blocks[0]).toMatchObject({ stepCount: 2, failedCount: 1 });
	});

	it("returns no blocks for text-only conversations", () => {
		const { blocks } = group([
			user("Hi"),
			message("assistant", [text("Hello.")]),
		]);
		expect(blocks).toEqual([]);
	});

	it("folds a single step", () => {
		const { blocks } = group([user("Go"), ...step("a", 1, 2)]);
		expect(blocks).toHaveLength(1);
		expect(blocks[0].stepCount).toBe(1);
	});

	it("uses the wall-clock span of parallel tools rather than their sum", () => {
		const prompt = user("Go");
		const parallel = message(
			"assistant",
			[call("a", at(1)), call("b", at(1))],
			at(1),
		);
		const results = message(
			"tool",
			[result("a", at(11)), result("b", at(11))],
			at(11),
		);
		const { blocks } = group([prompt, parallel, results]);

		expect(blocks[0]).toMatchObject({
			stepCount: 2,
			startedAt: WORKING_FIXTURE_START + 1000,
			endedAt: WORKING_FIXTURE_START + 11_000,
		});
	});

	it("reads result timestamps from hidden tool messages", () => {
		const prompt = user("Go");
		const steps = step("a", 1, 30);
		const answer = message("assistant", [text("Done.")], at(31));
		const { blocks } = group([prompt, ...steps, answer]);
		expect(blocks[0].endedAt).toBe(WORKING_FIXTURE_START + 30_000);
	});

	it("spans merged read_file rows as one block", () => {
		const prompt = user("Go");
		const reads = [
			message("assistant", [call("r1", at(1), "read_file")], at(1)),
			message("tool", [result("r1", at(2), { name: "read_file" })], at(2)),
			message("assistant", [call("r2", at(3), "read_file")], at(3)),
			message("tool", [result("r2", at(4), { name: "read_file" })], at(4)),
		];
		const { rows, blocks } = group([prompt, ...reads]);

		// The merged read_file group is one timeline row.
		expect(rows).toHaveLength(2);
		expect(blocks).toHaveLength(1);
		expect(blocks[0]).toMatchObject({
			stepCount: 2,
			startedAt: WORKING_FIXTURE_START + 1000,
			endedAt: WORKING_FIXTURE_START + 4000,
		});
	});

	it("reports no duration when parts carry no timestamps", () => {
		const prompt = user("Go");
		const steps = [...step("a"), ...step("b")];
		const { blocks } = group([prompt, ...steps]);
		expect(blocks[0].startedAt).toBeUndefined();
		expect(blocks[0].endedAt).toBeUndefined();
		expect(blocks[0].stepCount).toBe(2);
	});

	it("excludes the idle gap before the next user message", () => {
		const prompt = user("Go");
		const steps = step("a", 1, 2);
		const nextPrompt = message("user", [text("Later")], at(60 * 60 * 24));
		const { blocks } = group([prompt, ...steps, nextPrompt]);
		expect(blocks[0].endedAt).toBe(WORKING_FIXTURE_START + 2000);
	});

	it("marks the oldest loaded block partial while older history exists and keeps its key across a prepend", () => {
		const prompt = user("Go");
		const steps = [...step("a", 1, 2), ...step("b", 3, 4), ...step("c", 5, 6)];
		const answer = message("assistant", [text("Done.")], at(7));
		const all = [prompt, ...steps, answer];

		const newest = group(all.slice(4), { hasMoreMessages: true });
		expect(newest.blocks).toHaveLength(1);
		expect(newest.blocks[0]).toMatchObject({
			isPartial: true,
			stepCount: 1,
			startedAt: WORKING_FIXTURE_START + 5000,
			endedAt: WORKING_FIXTURE_START + 6000,
			liveKey: "working:live:head:0",
		});

		const complete = group(all, { hasMoreMessages: false });
		expect(complete.blocks[0]).toMatchObject({
			isPartial: false,
			stepCount: 3,
			startedAt: WORKING_FIXTURE_START + 1000,
			endedAt: WORKING_FIXTURE_START + 6000,
			liveKey: `working:live:message:${prompt.id}:0`,
		});
		expect(complete.blocks[0].key).toBe(newest.blocks[0].key);
	});

	it("does not mark a block partial when the loaded history starts at its prompt", () => {
		const prompt = user("Go");
		const steps = step("a", 1, 2);
		const { blocks } = group([prompt, ...steps], { hasMoreMessages: true });
		expect(blocks[0].isPartial).toBe(false);
	});

	describe("live turns", () => {
		const liveOptions = (
			parts: TypesGen.ChatMessagePart[],
		): Pick<
			GroupWorkingBlocksOptions,
			"liveBlocks" | "liveTools" | "streamState"
		> => {
			const { streamState, streamTools } = buildStreamRenderState(parts);
			return {
				liveBlocks: streamState?.blocks ?? [],
				liveTools: streamTools,
				streamState,
			};
		};

		it("keys the live block by its turn and folds a streaming tool row into it", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const live = liveOptions([call("b", at(3))]);
			const { rows, blocks } = group([prompt, ...steps], {
				isTurnActive: true,
				isLiveRowCollapsible: true,
				...live,
			});

			expect(blocks).toHaveLength(1);
			expect(rowIds(rows, blocks[0].rowIndices)).toEqual([steps[0].id, "live"]);
			expect(blocks[0]).toMatchObject({
				isLive: true,
				stepCount: 2,
				startedAt: WORKING_FIXTURE_START + 1000,
				endedAt: undefined,
				key: `working:live:message:${prompt.id}:0`,
			});
		});

		it("keeps the block live while the final answer streams outside it", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const live = liveOptions([text("Here is what I found")]);
			const { rows, blocks } = group([prompt, ...steps], {
				isTurnActive: true,
				isLiveRowCollapsible: true,
				...live,
			});

			expect(blocks).toHaveLength(1);
			expect(rowIds(rows, blocks[0].rowIndices)).toEqual([steps[0].id]);
			expect(blocks[0].isLive).toBe(true);
		});

		it("folds an idle live row into the block it follows", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const live = liveOptions([]);
			const { rows, blocks } = group([prompt, ...steps], {
				isTurnActive: true,
				isLiveRowCollapsible: true,
				...live,
			});

			expect(blocks).toHaveLength(1);
			expect(rowIds(rows, blocks[0].rowIndices)).toEqual([steps[0].id, "live"]);
			expect(blocks[0]).toMatchObject({ isLive: true, stepCount: 1 });
		});

		it("does not start a block from an idle live row", () => {
			const live = liveOptions([]);
			const { blocks } = group([user("Go")], {
				isTurnActive: true,
				isLiveRowCollapsible: true,
				...live,
			});
			expect(blocks).toEqual([]);
		});

		it("folds the live turn's reasoning before its first tool call", () => {
			const prompt = user("Go");
			const live = liveOptions([reasoning("Planning", at(1))]);
			const { rows, blocks } = group([prompt], {
				isTurnActive: true,
				isLiveRowCollapsible: true,
				...live,
			});

			expect(blocks).toHaveLength(1);
			expect(rowIds(rows, blocks[0].rowIndices)).toEqual(["live"]);
			expect(blocks[0]).toMatchObject({
				isLive: true,
				stepCount: 0,
				startedAt: WORKING_FIXTURE_START + 1000,
				key: `working:live:message:${prompt.id}:0`,
			});
		});

		it("keeps the live row outside the block when its callouts must stay visible", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const live = liveOptions([call("b", at(3))]);
			const { rows, blocks } = group([prompt, ...steps], {
				isTurnActive: true,
				isLiveRowCollapsible: false,
				...live,
			});

			expect(rowIds(rows, blocks[0].rowIndices)).toEqual([steps[0].id]);
			expect(blocks[0].isLive).toBe(true);
		});

		it("starts the live clock from streamed tool timestamps before anything persists", () => {
			const prompt = user("Go");
			const live = liveOptions([call("a", at(2))]);
			const { blocks } = group([prompt], {
				isTurnActive: true,
				isLiveRowCollapsible: true,
				...live,
			});
			expect(blocks).toHaveLength(1);
			expect(blocks[0].startedAt).toBe(WORKING_FIXTURE_START + 2000);
		});

		it("hands the live block off to a completed block that shares its liveKey", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const live = liveOptions([call("b", at(3))]);
			const running = group([prompt, ...steps], {
				isTurnActive: true,
				isLiveRowCollapsible: true,
				...live,
			});
			const persisted = [...steps, ...step("b", 3, 4)];
			const answer = message("assistant", [text("Done.")], at(5));
			const done = group([prompt, ...persisted, answer]);

			expect(done.blocks[0].liveKey).toBe(running.blocks[0].liveKey);
			expect(done.blocks[0].key).not.toBe(running.blocks[0].key);
			expect(done.blocks[0]).toMatchObject({
				isLive: false,
				startedAt: WORKING_FIXTURE_START + 1000,
				endedAt: WORKING_FIXTURE_START + 4000,
			});
		});

		it("stays live between persisted steps when no stream row exists", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const pending = message("assistant", [call("b", at(3))], at(3));
			const entries = parseMessagesWithMergedTools(
				[prompt, ...steps, pending],
				{
					pendingToolCallIDs: new Set(["b"]),
				},
			);
			const rows = assignTimelineRows(buildDisplayMessages(entries), false);
			const blocks = groupWorkingBlocks(rows, entries, {
				...defaultOptions,
				isTurnActive: true,
			});

			expect(blocks).toHaveLength(1);
			expect(blocks[0]).toMatchObject({
				isLive: true,
				stepCount: 2,
				startedAt: WORKING_FIXTURE_START + 1000,
				endedAt: undefined,
			});
		});

		it("keeps a tool awaiting a client out of the fold while the turn is parked", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const parked = message(
				"assistant",
				[call("b", at(3), "open_editor")],
				at(3),
			);
			const entries = parseMessagesWithMergedTools([prompt, ...steps, parked], {
				pendingToolCallIDs: new Set(["b"]),
			});
			const rows = assignTimelineRows(buildDisplayMessages(entries), false);
			const blocks = groupWorkingBlocks(rows, entries, defaultOptions);

			expect(blocks).toHaveLength(1);
			expect(rowIds(rows, blocks[0].rowIndices)).toEqual([steps[0].id]);
			expect(blocks[0]).toMatchObject({
				isLive: false,
				stepCount: 1,
				endedAt: WORKING_FIXTURE_START + 2000,
			});
		});

		it("gives a prompt-less live block a stable head liveKey", () => {
			const steps = [...step("a", 1, 2), ...step("b", 3, 4)];
			const live = liveOptions([call("c", at(5))]);
			const running = group(steps, {
				hasMoreMessages: true,
				isTurnActive: true,
				isLiveRowCollapsible: true,
				...live,
			});
			expect(running.blocks[0]).toMatchObject({
				isLive: true,
				isPartial: true,
				key: "working:live:head:0",
				liveKey: "working:live:head:0",
			});
			const done = group([...steps, ...step("c", 5, 6)], {
				hasMoreMessages: true,
			});
			expect(done.blocks[0].liveKey).toBe("working:live:head:0");
			expect(done.blocks[0].isPartial).toBe(true);
		});

		it("does not treat a completed earlier turn as live", () => {
			const first = user("One");
			const firstSteps = step("a", 1, 2);
			const firstAnswer = message("assistant", [text("Done one.")], at(3));
			const second = user("Two");
			const live = liveOptions([call("b", at(5))]);
			const { blocks } = group([first, ...firstSteps, firstAnswer, second], {
				isTurnActive: true,
				isLiveRowCollapsible: true,
				...live,
			});

			expect(blocks).toHaveLength(2);
			expect(blocks[0].isLive).toBe(false);
			expect(blocks[1].isLive).toBe(true);
		});
	});
});

describe("formatWorkingDuration", () => {
	it.each([
		[0, "0s"],
		[999, "0s"],
		[12_000, "12s"],
		[60_000, "1m 0s"],
		[134_000, "2m 14s"],
		[3_600_000, "1h 0m"],
		[3_780_000, "1h 3m"],
		[-5000, "0s"],
	])("formats %d ms as %s", (milliseconds, expected) => {
		expect(formatWorkingDuration(milliseconds)).toBe(expected);
	});
});
