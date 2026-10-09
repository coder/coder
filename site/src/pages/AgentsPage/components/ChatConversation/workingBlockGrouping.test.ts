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
	type GroupWorkingBlocksOptions,
	groupWorkingBlocks,
	splitRowBlocks,
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
const source = (url: string, title: string): TypesGen.ChatMessagePart => ({
	type: "source",
	url,
	title,
});
const citation = source("https://go.example.com", "Go");
const searchCall = (
	id: string,
	createdAt?: string,
): TypesGen.ChatMessagePart => ({
	type: "tool-call",
	tool_call_id: id,
	tool_name: "web_search",
	provider_executed: true,
	created_at: createdAt,
});
const searchResult = (id: string): TypesGen.ChatMessagePart => ({
	type: "tool-result",
	tool_call_id: id,
	tool_name: "web_search",
	provider_executed: true,
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
	isWorking: false,
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
			answer.id,
		]);
		// Reasoning start counts toward the wall-clock span.
		expect(blocks[0].startedAt).toBe(WORKING_FIXTURE_START + 500);
	});

	it("folds the final answer's reasoning and web search into the block it ends", () => {
		const prompt = user("Go");
		const steps = [...step("a", 1, 2), ...step("b", 3, 4)];
		const answer = message(
			"assistant",
			[
				reasoning("Check the docs", at(5), at(9)),
				source("https://example.com/a", "A"),
				source("https://example.com/b", "B"),
				text("Done."),
			],
			at(10),
		);
		const { rows, blocks } = group([prompt, ...steps, answer]);

		expect(blocks).toHaveLength(1);
		expect(rowIds(rows, blocks[0].rowIndices)).toEqual([
			steps[0].id,
			steps[2].id,
			answer.id,
		]);
		expect(blocks[0]).toMatchObject({
			stepCount: 3,
			endsWithAnswer: true,
			endedAt: WORKING_FIXTURE_START + 9000,
			key: `working:through:message:${answer.id}`,
		});
	});

	it("treats an answer whose citations trail its text as the block's answer", () => {
		const prompt = user("Go");
		const steps = step("a", 1, 2);
		const answer = message("assistant", [
			text("Done."),
			source("https://example.com", "Example"),
		]);
		const { blocks } = group([prompt, ...steps, answer]);

		expect(blocks[0]).toMatchObject({ stepCount: 2, endsWithAnswer: true });
	});

	it("starts a one-step block from an answer that searched in a turn without tools", () => {
		const prompt = user("Go");
		const answer = message("assistant", [
			reasoning("Look it up"),
			source("https://example.com", "Example"),
			text("Found it."),
		]);
		const { rows, blocks } = group([prompt, answer]);

		expect(blocks).toHaveLength(1);
		expect(rowIds(rows, blocks[0].rowIndices)).toEqual([answer.id]);
		expect(blocks[0]).toMatchObject({ stepCount: 1, endsWithAnswer: true });
	});

	it("counts a reissued search once, ignoring its unanswered first call", () => {
		const prompt = user("Go");
		const steps = step("a", 1, 2, { leading: [searchCall("unanswered")] });
		const answer = message("assistant", [
			searchCall("search"),
			citation,
			searchResult("search"),
			text("Go 1.27 is out."),
		]);
		const { blocks } = group([prompt, ...steps, answer]);

		expect(blocks).toMatchObject([{ stepCount: 2, endsWithAnswer: true }]);
	});

	it("leaves an answer's reasoning unfolded in a turn without steps", () => {
		const prompt = user("Go");
		const answer = message("assistant", [reasoning("Easy"), text("Done.")]);
		const { blocks } = group([prompt, answer]);

		expect(blocks).toEqual([]);
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

	it("folds a web search row that sits between tool steps", () => {
		const prompt = user("Go");
		const first = step("a", 1, 2);
		const search = message("assistant", [
			reasoning("Search"),
			source("https://example.com", "Example"),
		]);
		const second = step("b", 5, 6);
		const { rows, blocks } = group([prompt, ...first, search, ...second]);

		expect(blocks).toHaveLength(1);
		expect(rowIds(rows, blocks[0].rowIndices)).toEqual([
			first[0].id,
			search.id,
			second[0].id,
		]);
		expect(blocks[0]).toMatchObject({ stepCount: 3, endsWithAnswer: false });
	});

	it("starts a new block after an answer row closes one", () => {
		const prompt = user("Go");
		const first = step("a", 1, 2);
		const interlude = message("assistant", [
			reasoning("Halfway"),
			text("Halfway there."),
		]);
		const second = step("b", 4, 5);
		const { rows, blocks } = group([prompt, ...first, interlude, ...second]);

		expect(blocks).toHaveLength(2);
		expect(rowIds(rows, blocks[0].rowIndices)).toEqual([
			first[0].id,
			interlude.id,
		]);
		expect(blocks[0].endsWithAnswer).toBe(true);
		expect(rowIds(rows, blocks[1].rowIndices)).toEqual([second[0].id]);
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

	it("folds failed steps like any other step", () => {
		const prompt = user("Go");
		const steps = [...step("a", 1, 2, { isError: true }), ...step("b", 3, 4)];
		const { blocks } = group([prompt, ...steps]);

		expect(blocks).toHaveLength(1);
		expect(blocks[0]).toMatchObject({ stepCount: 2 });
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
			memberIds: [reads[0].id, reads[2].id],
			startedAt: WORKING_FIXTURE_START + 1000,
			endedAt: WORKING_FIXTURE_START + 4000,
		});
	});

	it("keeps a context-only user message inside the turn", () => {
		const prompt = user("Build it");
		const createWorkspace = [
			message("assistant", [call("w", at(1), "create_workspace")], at(1)),
			message(
				"tool",
				[result("w", at(2), { name: "create_workspace" })],
				at(2),
			),
		];
		const contextFiles = message(
			"user",
			[{ type: "context-file", context_file_path: "/AGENTS.md" }],
			at(2),
		);
		const steps = [...step("a", 3, 4), ...step("b", 5, 6)];
		const answer = message("assistant", [text("Done.")], at(7));
		const { rows, blocks } = group([
			prompt,
			...createWorkspace,
			contextFiles,
			...steps,
			answer,
		]);

		expect(blocks).toHaveLength(1);
		expect(rowIds(rows, blocks[0].rowIndices)).toEqual([
			createWorkspace[0].id,
			steps[0].id,
			steps[2].id,
		]);
		expect(blocks[0]).toMatchObject({
			stepCount: 3,
			startedAt: WORKING_FIXTURE_START + 1000,
			endedAt: WORKING_FIXTURE_START + 6000,
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
		const groupLive = (
			messages: readonly TypesGen.ChatMessage[],
			parts: TypesGen.ChatMessagePart[],
			options: Partial<GroupWorkingBlocksOptions> = {},
		) => {
			const { streamState, streamTools } = buildStreamRenderState(parts);

			return group(messages, {
				isTurnActive: true,
				isWorking: true,
				isLiveRowCollapsible: true,
				liveBlocks: streamState?.blocks ?? [],
				liveTools: streamTools,
				streamState,
				...options,
			});
		};

		it("keys the live block by its turn and folds a streaming tool row into it", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const { rows, blocks } = groupLive(
				[prompt, ...steps],
				[call("b", at(3))],
			);

			expect(blocks).toHaveLength(1);
			expect(rowIds(rows, blocks[0].rowIndices)).toEqual([steps[0].id, "live"]);
			expect(blocks[0]).toMatchObject({
				isLive: true,
				stepCount: 2,
				memberIds: [steps[0].id],
				startedAt: WORKING_FIXTURE_START + 1000,
				endedAt: undefined,
				key: `working:live:message:${prompt.id}:0`,
			});
		});

		it("keeps the block live while the final answer streams outside it", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const { rows, blocks } = groupLive(
				[prompt, ...steps],
				[text("Here is what I found")],
			);

			expect(blocks).toHaveLength(1);
			expect(rowIds(rows, blocks[0].rowIndices)).toEqual([steps[0].id]);
			expect(blocks[0].isLive).toBe(true);
		});

		it("keeps a live answer row with reasoning in the live block", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const { rows, blocks } = groupLive(
				[prompt, ...steps],
				[reasoning("Wrap up", at(3)), text("Here is what I found")],
			);

			expect(blocks).toHaveLength(1);
			expect(rowIds(rows, blocks[0].rowIndices)).toEqual([steps[0].id, "live"]);
			expect(blocks[0]).toMatchObject({ isLive: true, endsWithAnswer: true });
		});

		it("unfolds a tool-less live turn's reasoning once its answer starts", () => {
			const { blocks } = groupLive(
				[user("Go")],
				[reasoning("Planning", at(1)), text("Here you go")],
			);

			expect(blocks).toEqual([]);
		});

		// OpenAI streams a search's citations inside the answer text that follows
		// it, and stores them before that text.
		it.each([
			{
				name: "reasoning before the search",
				live: [
					reasoning("Look it up", at(1)),
					searchCall("a"),
					searchResult("a"),
					text("Go 1.27"),
					citation,
					text(" is out."),
				],
				persisted: [
					reasoning("Look it up", at(1), at(2)),
					searchCall("a"),
					searchResult("a"),
					citation,
					text("Go 1.27 is out."),
				],
				stepCount: 1,
			},
			{
				name: "the search before its reasoning",
				live: [
					searchCall("a", at(1)),
					searchResult("a"),
					reasoning("Compare", at(2)),
					text("Go 1.27"),
					citation,
					text(" is out."),
				],
				persisted: [
					searchCall("a", at(1)),
					searchResult("a"),
					reasoning("Compare", at(2), at(3)),
					citation,
					text("Go 1.27 is out."),
				],
				stepCount: 1,
			},
			{
				name: "narration before the search",
				live: [
					text("Searching."),
					searchCall("a", at(1)),
					searchResult("a"),
					text("Go 1.27"),
					citation,
					text(" is out."),
				],
				persisted: [
					text("Searching."),
					searchCall("a", at(1)),
					searchResult("a"),
					citation,
					text("Go 1.27 is out."),
				],
				stepCount: 1,
			},
			{
				name: "two searches",
				live: [
					reasoning("Look it up", at(1)),
					searchCall("a"),
					searchResult("a"),
					searchCall("b"),
					searchResult("b"),
					text("Go 1.27"),
					citation,
					text(" is out."),
				],
				persisted: [
					reasoning("Look it up", at(1), at(2)),
					searchCall("a"),
					searchResult("a"),
					searchCall("b"),
					searchResult("b"),
					citation,
					text("Go 1.27 is out."),
				],
				stepCount: 2,
			},
			{
				name: "a search without citations",
				live: [
					reasoning("Look it up", at(1)),
					searchCall("a"),
					searchResult("a"),
					text("Nothing new."),
				],
				persisted: [
					reasoning("Look it up", at(1), at(2)),
					searchCall("a"),
					searchResult("a"),
					text("Nothing new."),
				],
				stepCount: 1,
			},
		])(
			"keeps one search block through streaming and persistence with $name",
			({ live, persisted, stepCount }) => {
				const prompt = user("Go");
				const liveKey = `working:live:message:${prompt.id}:0`;
				const prefixBlocks = live.map(
					(_, index) => groupLive([prompt], live.slice(0, index + 1)).blocks,
				);
				const firstBlockIndex = prefixBlocks.findIndex(
					(blocks) => blocks.length > 0,
				);

				expect(firstBlockIndex).not.toBe(-1);
				for (const blocks of prefixBlocks.slice(firstBlockIndex)) {
					expect(blocks).toMatchObject([
						{
							key: liveKey,
							isLive: true,
							startedAt: WORKING_FIXTURE_START + 1000,
						},
					]);
				}
				expect(
					group([prompt, message("assistant", persisted)]).blocks,
				).toMatchObject([
					{
						liveKey,
						stepCount,
						endsWithAnswer: true,
						isLive: false,
						startedAt: WORKING_FIXTURE_START + 1000,
					},
				]);
			},
		);

		it("completes a block once its answer row persists while the chat still runs", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const answer = message(
				"assistant",
				[reasoning("Wrap up", at(3), at(4)), text("Done.")],
				at(5),
			);
			const { blocks } = group([prompt, ...steps, answer], { isWorking: true });

			expect(blocks).toHaveLength(1);
			expect(blocks[0]).toMatchObject({
				isLive: false,
				endedAt: WORKING_FIXTURE_START + 4000,
			});
		});

		it("folds an idle live row into the block it follows", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const { rows, blocks } = groupLive([prompt, ...steps], []);

			expect(blocks).toHaveLength(1);
			expect(rowIds(rows, blocks[0].rowIndices)).toEqual([steps[0].id, "live"]);
			expect(blocks[0]).toMatchObject({ isLive: true, stepCount: 1 });
		});

		it("does not start a block from an idle live row", () => {
			const { blocks } = groupLive([user("Go")], []);

			expect(blocks).toEqual([]);
		});

		it("folds the live turn's reasoning before its first tool call", () => {
			const prompt = user("Go");
			const { rows, blocks } = groupLive(
				[prompt],
				[reasoning("Planning", at(1))],
			);

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
			const { rows, blocks } = groupLive(
				[prompt, ...steps],
				[call("b", at(3))],
				{ isLiveRowCollapsible: false },
			);

			expect(rowIds(rows, blocks[0].rowIndices)).toEqual([steps[0].id]);
			expect(blocks[0].isLive).toBe(true);
		});

		it("hands the live block off to a completed block that shares its liveKey", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const running = groupLive([prompt, ...steps], [call("b", at(3))]);
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
				isWorking: true,
			});

			expect(blocks).toHaveLength(1);
			expect(blocks[0]).toMatchObject({
				isLive: true,
				stepCount: 2,
				startedAt: WORKING_FIXTURE_START + 1000,
				endedAt: undefined,
			});
		});

		it("stops the block's clock while an interrupt drains the turn", () => {
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
				isLive: false,
				stepCount: 2,
				endedAt: WORKING_FIXTURE_START + 3000,
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
			const running = groupLive(steps, [call("c", at(5))], {
				hasMoreMessages: true,
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
			const { blocks } = groupLive(
				[first, ...firstSteps, firstAnswer, second],
				[call("b", at(5))],
			);

			expect(blocks).toHaveLength(2);
			expect(blocks[0].isLive).toBe(false);
			expect(blocks[1].isLive).toBe(true);
		});
	});
});

describe("splitRowBlocks", () => {
	const answerOf = (content: TypesGen.ChatMessagePart[]) => {
		const [{ parsed }] = parseMessagesWithMergedTools([
			message("assistant", content),
		]);
		return splitRowBlocks(parsed.blocks, parsed.tools).answer;
	};

	it("folds narration that precedes the answer's last reasoning", () => {
		expect(
			answerOf([
				reasoning("Plan"),
				text("Looking it up."),
				source("https://example.com", "Example"),
				reasoning("Compare"),
				text("Done."),
			]),
		).toEqual([{ type: "response", text: "Done." }]);
	});

	// Persisted rows store text at its end, after the citations streamed in it.
	it.each([
		{
			name: "narration before search results",
			persisted: [
				text("Searching."),
				searchCall("a"),
				citation,
				searchResult("a"),
				text("Go 1.27 is out."),
			],
			live: [
				text("Searching."),
				searchCall("a"),
				citation,
				searchResult("a"),
				text("Go 1.27 is out."),
			],
			answer: "Go 1.27 is out.",
		},
		{
			name: "narration before cited text",
			persisted: [
				text("Searching."),
				searchCall("a"),
				searchResult("a"),
				citation,
				text("Go 1.27 is out. Rust 1.98 is out."),
			],
			live: [
				text("Searching."),
				searchCall("a"),
				searchResult("a"),
				text("Go 1.27 is out."),
				citation,
				text(" Rust 1.98 is out."),
			],
			answer: "Go 1.27 is out. Rust 1.98 is out.",
		},
		{
			name: "cited text before a search",
			persisted: [
				citation,
				text("Go 1.27 is out."),
				searchCall("a"),
				searchResult("a"),
				text("Rust 1.98 is out."),
			],
			live: [
				text("Go 1.27 is out."),
				citation,
				searchCall("a"),
				searchResult("a"),
				text("Rust 1.98 is out."),
			],
			answer: "Rust 1.98 is out.",
		},
		{
			name: "cited text without a search call",
			persisted: [citation, text("Go 1.27 is out. Rust 1.98 is out.")],
			live: [text("Go 1.27 is out."), citation, text(" Rust 1.98 is out.")],
			answer: "Go 1.27 is out. Rust 1.98 is out.",
		},
	])(
		"splits $name the same live and persisted",
		({ persisted, live, answer }) => {
			const { streamState, streamTools } = buildStreamRenderState(live);
			if (!streamState) {
				throw new Error("The live parts built no stream state.");
			}
			const expected = [{ type: "response", text: answer }];

			expect(answerOf(persisted)).toEqual(expected);
			expect(splitRowBlocks(streamState.blocks, streamTools).answer).toEqual(
				expected,
			);
		},
	);

	const hiddenCall = (id: string): TypesGen.ChatMessagePart => ({
		type: "tool-call",
		tool_call_id: id,
		tool_name: "execute",
		args: { command: "" },
	});

	it("ignores hidden tools when splitting the answer", () => {
		expect(
			answerOf([
				reasoning("Plan"),
				hiddenCall("before"),
				text("Done."),
				hiddenCall("after"),
			]),
		).toEqual([{ type: "response", text: "Done." }]);
	});

	it("keeps answer text on either side of a hidden tool apart", () => {
		expect(
			answerOf([
				reasoning("Plan"),
				text("Checked the logs."),
				hiddenCall("between"),
				text("The fix is in."),
			]),
		).toEqual([
			{ type: "response", text: "Checked the logs." },
			{ type: "response", text: "The fix is in." },
		]);
	});
});
