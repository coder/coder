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
import type { RenderBlock } from "./types";
import {
	type GroupWorkingBlocksOptions,
	getBlockTurnAttachments,
	getSliceContent,
	getTurnAttachmentsByRow,
	groupWorkingBlocks,
	isAgentAttachmentOnlyRow,
	type RowSlice,
	splitRowPieces,
	type TurnAttachmentsOptions,
	type WorkingBlockLayout,
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
const narration = (value: string): TypesGen.ChatMessagePart => ({
	type: "text",
	text: value,
	narration: true,
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
const searchResult = (
	id: string,
	createdAt?: string,
): TypesGen.ChatMessagePart => ({
	type: "tool-result",
	tool_call_id: id,
	tool_name: "web_search",
	provider_executed: true,
	created_at: createdAt,
});
// A provider-executed call that, unlike web_search, renders no row.
const hiddenCall = (
	id: string,
	createdAt?: string,
): TypesGen.ChatMessagePart => ({
	type: "tool-call",
	tool_call_id: id,
	tool_name: "code_execution",
	provider_executed: true,
	created_at: createdAt,
});
const hiddenResult = (
	id: string,
	createdAt?: string,
): TypesGen.ChatMessagePart => ({
	type: "tool-result",
	tool_call_id: id,
	tool_name: "code_execution",
	provider_executed: true,
	created_at: createdAt,
});
const call = (
	id: string,
	createdAt?: string,
	name = "execute",
	args: Record<string, string> = { command: `echo ${id}` },
): TypesGen.ChatMessagePart => ({
	type: "tool-call",
	tool_call_id: id,
	tool_name: name,
	args,
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

const mockAttachment: TypesGen.ChatMessagePart = {
	type: "file",
	media_type: "text/csv",
	name: "report.csv",
	file_id: "report-file",
};

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

const attachStep = (
	id: string,
	callAt: number,
	resultAt: number,
): TypesGen.ChatMessage[] => [
	message("assistant", [
		call(id, at(callAt), "attach_file", { path: "/home/coder/report.md" }),
	]),
	message("assistant", [
		{
			type: "file",
			media_type: "text/markdown",
			name: "report.md",
			file_id: `file-${id}`,
		},
	]),
	message("tool", [result(id, at(resultAt), { name: "attach_file" })]),
];

const defaultOptions: GroupWorkingBlocksOptions = {
	hasMoreMessages: false,
	isTurnActive: false,
	isTurnParked: false,
	isWorking: false,
	isLiveRowCollapsible: false,
	liveBlocks: [],
	liveTools: [],
};

type GroupedLayout = WorkingBlockLayout & { rows: readonly TimelineRow[] };

const group = (
	messages: readonly TypesGen.ChatMessage[],
	options: Partial<GroupWorkingBlocksOptions> = {},
): GroupedLayout => {
	const entries = parseMessagesWithMergedTools(messages);
	const hasLive = options.isTurnActive ?? false;
	const rows = assignTimelineRows(buildDisplayMessages(entries), hasLive);

	return {
		rows,
		...groupWorkingBlocks(rows, entries, {
			...defaultOptions,
			...options,
		}),
	};
};

const rowId = (row: TimelineRow) =>
	row.type === "live" ? "live" : row.entry.message.id;

const rowIds = (rows: readonly TimelineRow[], slices: readonly RowSlice[]) =>
	slices.map(({ rowIndex }) => rowId(rows[rowIndex]));

/**
 * The rendered order: message IDs, with the first piece of each part of a
 * split row, and each block as a nested list.
 */
const layoutOf = ({ rows, items }: GroupedLayout) => {
	const splitRows = new Set(
		items.flatMap((item) =>
			item.type === "slice" && item.slice.pieceRange
				? [item.slice.rowIndex]
				: [],
		),
	);
	const formatSlice = ({ rowIndex, pieceRange }: RowSlice) => {
		const id = `${rowId(rows[rowIndex])}`;
		return splitRows.has(rowIndex) && pieceRange
			? `${id}[${pieceRange.start}]`
			: id;
	};
	return items.map((item) =>
		item.type === "block"
			? item.block.slices.map(formatSlice)
			: formatSlice(item.slice),
	);
};

beforeEach(() => {
	nextId = 1;
});

describe("groupWorkingBlocks", () => {
	it("folds a turn's tool steps and leaves the answer and prompt visible", () => {
		const prompt = user("Inspect");
		const steps = [...step("a", 1, 4), ...step("b", 5, 13)];
		const answer = message("assistant", [text("Done.")], at(14));
		const layout = group([prompt, ...steps, answer]);
		const { rows, blocks } = layout;

		expect(layoutOf(layout)).toEqual([
			`${prompt.id}`,
			[`${steps[0].id}`, `${steps[2].id}`],
			`${answer.id}`,
		]);
		const [block] = blocks;
		expect(rowIds(rows, block.slices)).toEqual([steps[0].id, steps[2].id]);
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

	const textBetweenSteps = () => {
		const prompt = user("Go");
		const steps = [
			...step("a", 1, 2, {
				leading: [reasoning("Plan", at(0.5), at(1))],
			}),
			...step("b", 3, 4, { leading: [text("Let me check the tests.")] }),
		];
		const answer = message("assistant", [
			reasoning("Wrap up", at(5), at(6)),
			text("Done."),
		]);
		return { prompt, steps, answer, messages: [prompt, ...steps, answer] };
	};

	it("closes the block at text before a tool call while the turn is parked", () => {
		const { prompt, steps, answer, messages } = textBetweenSteps();
		const layout = group(messages, { isTurnParked: true });
		const [firstStepId, secondStepId] = [steps[0].id, steps[2].id];

		expect(layoutOf(layout)).toEqual([
			`${prompt.id}`,
			[`${firstStepId}`],
			`${secondStepId}[0]`,
			[`${secondStepId}[1]`, `${answer.id}[0]`],
			`${answer.id}[1]`,
		]);
		// Reasoning start counts toward the wall-clock span of its block.
		expect(layout.blocks).toMatchObject([
			{
				stepCount: 1,
				startedAt: WORKING_FIXTURE_START + 500,
				endedAt: WORKING_FIXTURE_START + 2000,
				key: `working:through:message:${firstStepId}`,
				liveKey: `working:live:message:${prompt.id}:0`,
			},
			{
				stepCount: 1,
				startedAt: WORKING_FIXTURE_START + 3000,
				endedAt: WORKING_FIXTURE_START + 6000,
				key: `working:through:message:${answer.id}`,
				liveKey: `working:live:message:${prompt.id}:1`,
			},
		]);
	});

	it("folds an ended turn's text between steps into one block and leaves only the final answer outside", () => {
		const { prompt, steps, answer, messages } = textBetweenSteps();
		const layout = group(messages);

		expect(layoutOf(layout)).toEqual([
			`${prompt.id}`,
			[`${steps[0].id}`, `${steps[2].id}`, `${answer.id}[0]`],
			`${answer.id}[1]`,
		]);
		expect(layout.blocks).toMatchObject([
			{
				stepCount: 2,
				startedAt: WORKING_FIXTURE_START + 500,
				endedAt: WORKING_FIXTURE_START + 6000,
				key: `working:through:message:${answer.id}`,
				liveKey: `working:live:message:${prompt.id}:0`,
			},
		]);
	});

	const callsAroundTextTurn = () => {
		const prompt = user("Go");
		const callsAroundText = message("assistant", [
			call("a", at(1)),
			text("Now the tests."),
			call("b", at(2)),
		]);
		const results = message("tool", [result("a", at(3)), result("b", at(4))]);
		return {
			prompt,
			callsAroundText,
			messages: [prompt, callsAroundText, results],
		};
	};

	it("keys the blocks on either side of text inside one step apart", () => {
		const { prompt, callsAroundText, messages } = callsAroundTextTurn();
		const layout = group(messages, { isTurnParked: true });

		expect(layoutOf(layout)).toEqual([
			`${prompt.id}`,
			[`${callsAroundText.id}[0]`],
			`${callsAroundText.id}[1]`,
			[`${callsAroundText.id}[2]`],
		]);
		expect(layout.blocks).toMatchObject([
			{
				key: `working:through:message:${callsAroundText.id}`,
				pieceKeys: [`${callsAroundText.id}:0`],
				startedAt: WORKING_FIXTURE_START + 1000,
				endedAt: WORKING_FIXTURE_START + 3000,
			},
			{
				key: `working:through:message:${callsAroundText.id}:1`,
				pieceKeys: [`${callsAroundText.id}:1`],
				startedAt: WORKING_FIXTURE_START + 2000,
				endedAt: WORKING_FIXTURE_START + 4000,
			},
		]);
	});

	it("keeps the running turn's last block key and piece keys once the turn ends and folds", () => {
		const { callsAroundText, messages } = callsAroundTextTurn();
		const running = group(messages, { isTurnActive: true });
		const ended = group(messages);

		expect(running.blocks).toMatchObject([
			{ turnKey: undefined },
			{ turnKey: undefined },
		]);
		expect(ended.blocks).toMatchObject([
			{
				key: running.blocks[1].key,
				turnKey: `working:turn:message:${callsAroundText.id}:1`,
				pieceKeys: [`${callsAroundText.id}:0`, `${callsAroundText.id}:1`],
				slices: [{ key: `message:${callsAroundText.id}` }],
			},
		]);
	});

	it("keeps an answer's key once its turn ends and folds the text before it in its row", () => {
		const prompt = user("Go");
		const answer = message("assistant", [
			text("Looking it up."),
			searchCall("s", at(1)),
			searchResult("s", at(2)),
			source("https://example.com/a", "A"),
			text("Found it."),
		]);
		const parked = group([prompt, answer], { isTurnParked: true });
		const ended = group([prompt, answer]);

		expect(layoutOf(parked)).toEqual([
			`${prompt.id}`,
			`${answer.id}[0]`,
			[`${answer.id}[1]`],
			`${answer.id}[2]`,
		]);
		expect(layoutOf(ended)).toEqual([
			`${prompt.id}`,
			[`${answer.id}[0]`],
			`${answer.id}[2]`,
		]);
		expect(ended.items.at(-1)).toEqual(parked.items.at(-1));
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
		expect(rowIds(rows, blocks[0].slices)).toEqual([
			steps[0].id,
			steps[2].id,
			answer.id,
		]);
		expect(blocks[0]).toMatchObject({
			stepCount: 3,
			endedAt: WORKING_FIXTURE_START + 9000,
			key: `working:through:message:${answer.id}`,
		});
	});

	it("keeps a step's attachment inside the block instead of ending it there", () => {
		const prompt = user("Go");
		// chatd persists a tool's attachments as their own assistant message.
		const steps = [
			...step("a", 1, 2),
			message("assistant", [call("b", at(3), "attach_file")], at(3)),
			message("assistant", [mockAttachment], at(3)),
			message("tool", [result("b", at(4), { name: "attach_file" })], at(4)),
			...step("c", 5, 6),
		];
		const answer = message("assistant", [text("Done.")], at(7));
		const layout = group([prompt, ...steps, answer]);

		expect(layoutOf(layout)).toEqual([
			`${prompt.id}`,
			[`${steps[0].id}`, `${steps[2].id}`, `${steps[3].id}`, `${steps[5].id}`],
			`${answer.id}`,
		]);
	});

	it("treats an answer whose citations trail its text as the block's answer", () => {
		const prompt = user("Go");
		const steps = step("a", 1, 2);
		const answer = message("assistant", [
			text("Done."),
			source("https://example.com", "Example"),
		]);
		const { blocks } = group([prompt, ...steps, answer]);

		expect(blocks[0]).toMatchObject({ stepCount: 2 });
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
		expect(rowIds(rows, blocks[0].slices)).toEqual([answer.id]);
		expect(blocks[0]).toMatchObject({ stepCount: 1 });
	});

	it("counts a reissued hidden call once, ignoring its unanswered first call", () => {
		const prompt = user("Go");
		const steps = step("a", 1, 2, { leading: [hiddenCall("unanswered")] });
		const answer = message("assistant", [
			hiddenCall("search"),
			citation,
			hiddenResult("search"),
			text("Go 1.27 is out."),
		]);
		const { blocks } = group([prompt, ...steps, answer]);

		expect(blocks).toMatchObject([{ stepCount: 2 }]);
	});

	it.each([
		{ name: "no answer", answers: [] },
		{
			name: "a cited answer",
			answers: [[text("Go 1.27"), citation, text(" is out."), citation]],
		},
	])(
		"keeps the step of a search an interrupt committed apart from its row with $name",
		({ answers }) => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			// chatd commits the call and result as separate messages that render
			// no row.
			const search = [
				message("assistant", [searchCall("s", at(3))]),
				message("tool", [searchResult("s", at(4))]),
			];
			const after = answers.map((parts) => message("assistant", parts));

			expect(
				group([prompt, ...steps, ...search, ...after]).blocks,
			).toMatchObject([{ stepCount: 2 }]);
		},
	);

	it("counts a hidden call that rendered no row before the block's first step", () => {
		const prompt = user("Go");
		// The answer's own call stays out of the block that follows it.
		const answer = message("assistant", [
			hiddenCall("first", at(1)),
			hiddenResult("first", at(2)),
			text("Nothing found."),
		]);
		const search = message("assistant", [
			hiddenCall("second", at(3)),
			hiddenResult("second", at(4)),
		]);
		const steps = step("a", 5, 6);

		expect(
			group([prompt, answer, search, ...steps], { isTurnParked: true }).blocks,
		).toMatchObject([
			{ stepCount: 2, startedAt: WORKING_FIXTURE_START + 3000 },
		]);
	});

	it("counts a search that rendered no row toward the ended turn's block that the next row's text starts", () => {
		const prompt = user("Go");
		const search = message("assistant", [
			searchCall("s", at(1)),
			searchResult("s", at(2)),
		]);
		const steps = step("a", 3, 4, { leading: [text("Found it, checking.")] });
		const answer = message("assistant", [text("Done.")]);

		expect(group([prompt, search, ...steps, answer]).blocks).toMatchObject([
			{ stepCount: 2, startedAt: WORKING_FIXTURE_START + 1000 },
		]);
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
		expect(rowIds(rows, blocks[0].slices)).toEqual([
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
		expect(rowIds(rows, blocks[0].slices)).toEqual([
			first[0].id,
			search.id,
			second[0].id,
		]);
		expect(blocks[0]).toMatchObject({ stepCount: 3 });
	});

	it("starts a new block after an answer row closes one", () => {
		const prompt = user("Go");
		const first = step("a", 1, 2);
		const interlude = message("assistant", [
			reasoning("Halfway"),
			text("Halfway there."),
		]);
		const second = step("b", 4, 5);
		const layout = group([prompt, ...first, interlude, ...second], {
			isTurnParked: true,
		});

		expect(layoutOf(layout)).toEqual([
			`${prompt.id}`,
			[`${first[0].id}`, `${interlude.id}[0]`],
			`${interlude.id}[1]`,
			[`${second[0].id}`],
		]);
	});

	it("counts a search between two blocks once", () => {
		const prompt = user("Go");
		const first = step("a", 1, 2);
		const interlude = message("assistant", [
			reasoning("Halfway"),
			text("Halfway there."),
		]);
		const search = [
			message("assistant", [searchCall("s", at(3))]),
			message("tool", [searchResult("s", at(4))]),
		];
		const second = step("b", 5, 6);
		const { blocks } = group(
			[prompt, ...first, interlude, ...search, ...second],
			{ isTurnParked: true },
		);

		expect(blocks).toHaveLength(2);
		expect(blocks.reduce((total, block) => total + block.stepCount, 0)).toBe(3);
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
			expect(blocks.flatMap((b) => rowIds(rows, b.slices))).toEqual([
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
		expect(rowIds(rows, blocks[0].slices)).toEqual([
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

	it("opens a partial block at the attached files that start the loaded history", () => {
		const prompt = user("Report");
		const [attachCall, files, attachResult] = attachStep("a", 1, 2);
		const steps = step("b", 3, 4);
		const answer = message("assistant", [text("Done.")], at(5));
		const all = [prompt, attachCall, files, attachResult, ...steps, answer];

		const newest = group(all.slice(2), { hasMoreMessages: true });
		expect(layoutOf(newest)).toEqual([
			[`${files.id}`, `${steps[0].id}`],
			`${answer.id}`,
		]);
		expect(newest.blocks[0]).toMatchObject({ isPartial: true, stepCount: 1 });

		expect(layoutOf(group(all))).toEqual([
			`${prompt.id}`,
			[`${attachCall.id}`, `${files.id}`, `${steps[0].id}`],
			`${answer.id}`,
		]);
	});

	it.each([
		{ name: "its prompt", opening: () => user("Go"), isTurnParked: false },
		{
			name: "an assistant text row of a parked turn",
			opening: () => message("assistant", [text("Next, the tests.")]),
			isTurnParked: true,
		},
	])(
		"does not mark a block partial when the loaded history starts at $name",
		({ opening, isTurnParked }) => {
			const { blocks } = group([opening(), ...step("a", 1, 2)], {
				hasMoreMessages: true,
				isTurnParked,
			});

			expect(blocks[0].isPartial).toBe(false);
		},
	);

	it("keeps a block partial through the text that starts an ended turn's loaded history", () => {
		const note = message("assistant", [text("Next, the tests.")]);
		const steps = step("a", 1, 2);
		const layout = group([note, ...steps], { hasMoreMessages: true });

		expect(layoutOf(layout)).toEqual([[`${note.id}`, `${steps[0].id}`]]);
		expect(layout.blocks[0].isPartial).toBe(true);
	});

	it("keeps an ended turn's text before a question visible and splits the turn there", () => {
		const prompt = user("Go");
		const before = step("a", 1, 2);
		const note = message("assistant", [text("I need your input.")]);
		const question = [
			message("assistant", [call("q", at(3), "ask_user_question")]),
			message("tool", [result("q", at(4), { name: "ask_user_question" })]),
		];
		const after = step("b", 5, 6);
		const answer = message("assistant", [text("Done.")]);
		const layout = group([
			prompt,
			...before,
			note,
			...question,
			...after,
			answer,
		]);

		expect(layoutOf(layout)).toEqual([
			`${prompt.id}`,
			[`${before[0].id}`],
			`${note.id}`,
			`${question[0].id}`,
			[`${after[0].id}`],
			`${answer.id}`,
		]);
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
			expect(rowIds(rows, blocks[0].slices)).toEqual([steps[0].id, "live"]);
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
			expect(rowIds(rows, blocks[0].slices)).toEqual([steps[0].id]);
			expect(blocks[0].isLive).toBe(true);
		});

		it("keeps a live answer row with reasoning in the live block", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const live = groupLive(
				[prompt, ...steps],
				[reasoning("Wrap up", at(3)), text("Here is what I found")],
			);

			expect(layoutOf(live)).toEqual([
				`${prompt.id}`,
				[`${steps[0].id}`, "live[0]"],
				"live[1]",
			]);
			expect(live.blocks[0]).toMatchObject({ isLive: true, stepCount: 1 });
		});

		it("counts an answer's hidden call without citations toward the block it closes", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const parts = [
				hiddenCall("s", at(3)),
				hiddenResult("s", at(4)),
				text("Nothing new."),
			];
			const answer = message("assistant", parts);
			const live = groupLive([prompt, ...steps], parts);
			const persisted = group([prompt, ...steps, answer]);

			expect(layoutOf(live)).toEqual([
				`${prompt.id}`,
				[`${steps[0].id}`],
				"live",
			]);
			expect(live.blocks).toMatchObject([{ stepCount: 2, isLive: true }]);
			expect(layoutOf(persisted)).toEqual([
				`${prompt.id}`,
				[`${steps[0].id}`],
				`${answer.id}`,
			]);
			expect(persisted.blocks).toMatchObject([
				{ stepCount: 2, endedAt: WORKING_FIXTURE_START + 4000 },
			]);
		});

		it("starts no block from a hidden call without citations alone", () => {
			const prompt = user("Go");
			const parts = [hiddenCall("s", at(1)), hiddenResult("s"), text("Hi.")];

			for (const index of parts.keys()) {
				expect(groupLive([prompt], parts.slice(0, index + 1)).blocks).toEqual(
					[],
				);
			}
			expect(group([prompt, message("assistant", parts)]).blocks).toEqual([]);
		});

		it("unfolds a tool-less live turn's reasoning once its answer starts", () => {
			const { blocks } = groupLive(
				[user("Go")],
				[reasoning("Planning", at(1)), text("Here you go")],
			);

			expect(blocks).toEqual([]);
		});

		it("streams text between steps outside the block and opens the next below it", () => {
			const prompt = user("Go");
			const steps = step("a", 1, 2);
			const parts = [text("P1 trials are running."), call("b", at(3))];
			const answer = message("assistant", parts);
			const streaming = groupLive([prompt, ...steps], parts.slice(0, 1));
			const next = groupLive([prompt, ...steps], parts);
			const persisted = groupLive(
				[prompt, ...steps, answer, message("tool", [result("b", at(4))])],
				[],
			);
			const first = `working:live:message:${prompt.id}:0`;
			const second = `working:live:message:${prompt.id}:1`;

			expect(layoutOf(streaming)).toEqual([
				`${prompt.id}`,
				[`${steps[0].id}`],
				"live",
			]);
			expect(streaming.blocks).toMatchObject([{ key: first, isLive: true }]);
			expect(layoutOf(next)).toEqual([
				`${prompt.id}`,
				[`${steps[0].id}`],
				"live[0]",
				["live[1]"],
			]);
			expect(next.blocks).toMatchObject([
				{
					liveKey: first,
					isLive: false,
					endedAt: WORKING_FIXTURE_START + 2000,
				},
				{
					key: second,
					isLive: true,
					liveWorkOrdinal: 0,
					pieceKeys: [],
					startedAt: WORKING_FIXTURE_START + 3000,
				},
			]);
			expect(
				next.items.map((item) =>
					item.type === "slice" ? item.slice.key : "block",
				),
			).toEqual([`message:${prompt.id}`, "block", "live-assistant", "block"]);
			expect(layoutOf(persisted)).toEqual([
				`${prompt.id}`,
				[`${steps[0].id}`],
				`${answer.id}[0]`,
				[`${answer.id}[1]`, "live"],
			]);
			expect(persisted.blocks).toMatchObject([
				{ liveKey: first, isLive: false },
				{ key: second, isLive: true, pieceKeys: [`${answer.id}:0`] },
			]);
		});

		it("renders labeled narration as output that closes the live block", () => {
			const prompt = user("Go");
			const parts = [
				reasoning("Planning", at(1)),
				narration("Now I'll "),
				narration("update the tests."),
				call("a", at(2)),
			];
			const live = groupLive([prompt], parts);

			expect(layoutOf(live)).toEqual([`${prompt.id}`, "live[0]", ["live[2]"]]);
			expect(live.blocks).toMatchObject([
				{
					key: `working:live:message:${prompt.id}:1`,
					stepCount: 1,
					liveWorkOrdinal: 1,
				},
			]);
		});

		it("folds a tool-less turn's labeled narration once the turn ends, not while it streams", () => {
			const prompt = user("Go");
			const parts = [narration("Checking the docs."), text("They cover it.")];
			const answer = message("assistant", parts);
			const live = groupLive([prompt], parts);
			const persisted = group([prompt, answer]);

			expect(live.blocks).toEqual([]);
			expect(layoutOf(persisted)).toEqual([
				`${prompt.id}`,
				[`${answer.id}[0]`],
				`${answer.id}[1]`,
			]);
			expect(persisted.blocks).toMatchObject([
				{ stepCount: 0, key: `working:through:message:${answer.id}` },
			]);
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
				name: "text before the search",
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
			const { blocks } = group([prompt, ...steps, answer], {
				isTurnActive: true,
				isWorking: true,
			});

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
			expect(rowIds(rows, blocks[0].slices)).toEqual([steps[0].id, "live"]);
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
			expect(rowIds(rows, blocks[0].slices)).toEqual(["live"]);
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

			expect(rowIds(rows, blocks[0].slices)).toEqual([steps[0].id]);
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
			const { blocks } = groupWorkingBlocks(rows, entries, {
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

		it("stays live while only files attached after its last step follow it", () => {
			const entries = parseMessagesWithMergedTools([
				user("Go"),
				...attachStep("a", 1, 2),
			]);
			const rows = assignTimelineRows(buildDisplayMessages(entries), false);
			const { blocks } = groupWorkingBlocks(rows, entries, {
				...defaultOptions,
				isTurnActive: true,
				isWorking: true,
			});

			expect(blocks).toMatchObject([{ isLive: true, endedAt: undefined }]);
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
			const { blocks } = groupWorkingBlocks(rows, entries, {
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
			const { blocks } = groupWorkingBlocks(rows, entries, {
				...defaultOptions,
				isTurnParked: true,
			});

			expect(blocks).toHaveLength(1);
			expect(rowIds(rows, blocks[0].slices)).toEqual([steps[0].id]);
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

		it("folds only earlier turns' text between steps, not the running turn's around a hook notice", () => {
			const first = user("One");
			const firstSteps = step("a", 1, 2);
			const note = message("assistant", [text("Halfway there.")]);
			const moreSteps = step("b", 3, 4);
			const firstAnswer = message("assistant", [text("Done one.")]);
			const second = user("Two");
			const secondSteps = step("c", 5, 6);
			const secondNote = message("assistant", [text("Still going.")]);
			const lastSteps = step("d", 7, 8);
			const hook = message("system", [
				{ type: "hook-notice", text: "Formatter ran." },
			]);
			const layout = groupLive(
				[
					first,
					...firstSteps,
					note,
					...moreSteps,
					firstAnswer,
					second,
					...secondSteps,
					secondNote,
					...lastSteps,
					hook,
				],
				[call("e", at(9))],
			);

			expect(layoutOf(layout)).toEqual([
				`${first.id}`,
				[`${firstSteps[0].id}`, `${note.id}`, `${moreSteps[0].id}`],
				`${firstAnswer.id}`,
				`${second.id}`,
				[`${secondSteps[0].id}`],
				`${secondNote.id}`,
				[`${lastSteps[0].id}`],
				`${hook.id}`,
				["live"],
			]);
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

describe("splitRowPieces", () => {
	const piecesOf = (content: TypesGen.ChatMessagePart[]) => {
		const [{ parsed }] = parseMessagesWithMergedTools([
			message("assistant", content),
		]);
		return splitRowPieces(parsed.blocks, parsed.tools).pieces;
	};
	const work = (...blocks: RenderBlock[]) => ({ kind: "work", blocks });
	const output = (...blocks: RenderBlock[]) => ({ kind: "output", blocks });
	const said = (value: string): RenderBlock => ({
		type: "response",
		text: value,
	});
	const thought = (value: string): RenderBlock => ({
		type: "thinking",
		text: value,
	});
	const cited: RenderBlock = {
		type: "sources",
		sources: [{ url: "https://go.example.com", title: "Go" }],
	};

	it("alternates work and output, folding a citation ahead of its text", () => {
		expect(
			piecesOf([
				reasoning("Plan"),
				text("Looking it up."),
				citation,
				reasoning("Compare"),
				text("Done."),
			]),
		).toEqual([
			work(thought("Plan"), cited),
			output(said("Looking it up.")),
			work(thought("Compare")),
			output(said("Done.")),
		]);
	});

	// Persisted rows store text at its end, after the citations streamed in it.
	it.each([
		{
			name: "text before hidden call results",
			persisted: [
				text("Searching."),
				hiddenCall("a"),
				citation,
				hiddenResult("a"),
				text("Go 1.27 is out."),
			],
			live: [
				text("Searching."),
				hiddenCall("a"),
				citation,
				hiddenResult("a"),
				text("Go 1.27 is out."),
			],
			pieces: [
				output({
					type: "response",
					text: "Searching.",
					beforeProviderTool: true,
				}),
				work(cited),
				output(said("Go 1.27 is out.")),
			],
		},
		{
			name: "text before a hidden call and cited text",
			persisted: [
				text("Searching."),
				hiddenCall("a"),
				hiddenResult("a"),
				citation,
				text("Go 1.27 is out. Rust 1.98 is out."),
			],
			live: [
				text("Searching."),
				hiddenCall("a"),
				hiddenResult("a"),
				text("Go 1.27 is out."),
				citation,
				text(" Rust 1.98 is out."),
			],
			pieces: [
				output({
					type: "response",
					text: "Searching.",
					beforeProviderTool: true,
				}),
				work(cited),
				output(said("Go 1.27 is out. Rust 1.98 is out.")),
			],
		},
		{
			name: "cited text without a search call",
			persisted: [citation, text("Go 1.27 is out. Rust 1.98 is out.")],
			live: [text("Go 1.27 is out."), citation, text(" Rust 1.98 is out.")],
			pieces: [work(cited), output(said("Go 1.27 is out. Rust 1.98 is out."))],
		},
		{
			name: "labeled narration before the answer",
			persisted: [narration("Checking the docs."), text("They cover it.")],
			live: [
				narration("Checking "),
				narration("the docs."),
				text("They cover it."),
			],
			pieces: [
				output({
					type: "response",
					text: "Checking the docs.",
					narration: true,
				}),
				output(said("They cover it.")),
			],
		},
	])(
		"splits $name the same live and persisted",
		({ persisted, live, pieces }) => {
			const { streamState, streamTools } = buildStreamRenderState(live);
			if (!streamState) {
				throw new Error("The live parts built no stream state.");
			}

			expect(piecesOf(persisted)).toEqual(pieces);
			expect(splitRowPieces(streamState.blocks, streamTools).pieces).toEqual(
				pieces,
			);
		},
	);

	const emptyExecuteCall = (id: string): TypesGen.ChatMessagePart => ({
		type: "tool-call",
		tool_call_id: id,
		tool_name: "execute",
		args: { command: "" },
	});

	it("leaves hidden tools out of the pieces", () => {
		expect(
			piecesOf([
				reasoning("Plan"),
				emptyExecuteCall("before"),
				text("Done."),
				emptyExecuteCall("after"),
			]),
		).toEqual([work(thought("Plan")), output(said("Done."))]);
	});

	it("keeps text on either side of a hidden tool apart", () => {
		expect(
			piecesOf([
				reasoning("Plan"),
				text("Checked the logs."),
				emptyExecuteCall("between"),
				text("The fix is in."),
			]),
		).toEqual([
			work(thought("Plan")),
			output(said("Checked the logs."), said("The fix is in.")),
		]);
	});
});

describe("getSliceContent", () => {
	it.each([
		{ name: "whole row", pieceRange: undefined },
		{ name: "slice", pieceRange: { start: 0, end: 2 } },
	])(
		"drops a $name's files only when attachments are hidden",
		({ pieceRange }) => {
			const [{ parsed }] = parseMessagesWithMergedTools([
				message("assistant", [text("Report:"), mockAttachment]),
			]);
			const blockTypes = (hideAttachments: boolean) =>
				getSliceContent(parsed.blocks, parsed.tools, {
					pieceRange,
					hideAttachments,
				}).blocks.map((block) => block.type);

			expect(blockTypes(false)).toEqual(["response", "file"]);
			expect(blockTypes(true)).toEqual(["response"]);
		},
	);
});

describe("isAgentAttachmentOnlyRow", () => {
	it("flags only assistant rows that show nothing but attachments", () => {
		const upload = message("user", [mockAttachment]);
		const attachmentOnly = message("assistant", [mockAttachment]);
		const answerWithFile = message("assistant", [
			text("Report:"),
			mockAttachment,
		]);
		const withHookNotice = message("assistant", [
			mockAttachment,
			{ type: "hook-notice", text: "Scanned by policy." },
		]);
		const { rows } = group([
			upload,
			attachmentOnly,
			answerWithFile,
			withHookNotice,
		]);

		expect(rows.map((row) => isAgentAttachmentOnlyRow(row))).toEqual([
			false,
			true,
			false,
			false,
		]);
	});
});

describe("getTurnAttachmentsByRow", () => {
	const file = (fileId: string): TypesGen.ChatMessagePart => ({
		...mockAttachment,
		file_id: fileId,
	});
	const mockIdleChat: TurnAttachmentsOptions = {
		hidesAttachmentRows: false,
		isChatActive: false,
		isCompacting: false,
	};
	const pills = (
		rows: readonly TimelineRow[],
		options: Partial<TurnAttachmentsOptions> = {},
	) =>
		[...getTurnAttachmentsByRow(rows, { ...mockIdleChat, ...options })].map(
			([key, attachments]) => [
				key,
				attachments.map(({ file }) => file.file_id),
			],
		);
	const attachSteps = (fileId: string) => [
		message("assistant", [call("attach", undefined, "attach_file")]),
		message("assistant", [file(fileId)]),
		message("tool", [result("attach", undefined, { name: "attach_file" })]),
	];

	it("lists each turn's agent files on its last assistant row", () => {
		const { rows } = group([
			user("Build the first report"),
			message("assistant", [text("Here:"), file("old")]),
			message("user", [text("Build the reports"), file("upload")]),
			message("assistant", [text("First:"), file("a")]),
			message("assistant", [file("b"), text("Then the second.")]),
			message("assistant", [text("Both reports are ready.")]),
		]);

		expect(pills(rows)).toEqual([
			["message:2", ["old"]],
			["message:6", ["a", "b"]],
		]);
	});

	it("keeps the pill of an earlier turn that stopped before answering", () => {
		const { rows } = group([
			user("Attach the report"),
			...attachSteps("a"),
			user("Continue"),
			message("assistant", [text("The report shows no errors.")]),
		]);

		for (const isChatActive of [true, false]) {
			expect(pills(rows, { hidesAttachmentRows: true, isChatActive })).toEqual([
				["message:2", ["a"]],
			]);
		}
	});

	it("gives a block the pill only when its turn ends without an answer", () => {
		const blockPills = ({ rows, blocks }: ReturnType<typeof group>) => {
			const byRow = getTurnAttachmentsByRow(rows, {
				...mockIdleChat,
				hidesAttachmentRows: true,
			});
			return blocks.map((block) =>
				getBlockTurnAttachments(block, rows, byRow)?.map(
					({ file }) => file.file_id,
				),
			);
		};

		expect(
			blockPills(group([user("Attach the report"), ...attachSteps("a")])),
		).toEqual([["a"]]);

		expect(
			blockPills(
				group([
					user("Attach the report"),
					...attachSteps("b"),
					message("assistant", [
						call("check", undefined, "execute"),
						text("Done."),
					]),
					message("tool", [result("check")]),
				]),
			),
		).toEqual([undefined]);

		const { streamState, streamTools } = buildStreamRenderState([
			reasoning("Thinking"),
			text("Sure"),
		]);
		expect(
			blockPills(
				group([user("Attach the report"), ...attachSteps("c")], {
					isTurnActive: true,
					isWorking: true,
					isLiveRowCollapsible: true,
					streamState,
					liveBlocks: streamState?.blocks ?? [],
					liveTools: streamTools,
				}),
			),
		).toEqual([["c"]]);
	});

	it("skips hidden attachment-only rows when picking the pill's row", () => {
		const { rows } = group([
			user("Build the report"),
			message("assistant", [text("Report attached.")]),
			message("assistant", [file("a")]),
		]);

		expect(pills(rows, { hidesAttachmentRows: true })).toEqual([
			["message:2", ["a"]],
		]);
		expect(pills(rows)).toEqual([["message:3", ["a"]]]);
	});

	it.each(["chat_cleared", "chat_summarized"])(
		"keeps the pill off a later %s row",
		(toolName) => {
			const { rows } = group([
				user("Build the report"),
				message("assistant", [text("Here:"), file("a")]),
				message("assistant", [call("boundary", undefined, toolName)]),
				message("tool", [result("boundary", undefined, { name: toolName })]),
			]);

			expect(pills(rows)).toEqual([["message:2", ["a"]]]);
		},
	);

	it("keeps the pill on the answer when a hook notice follows it", () => {
		const { rows } = group([
			user("Build the report"),
			message("assistant", [text("Here:"), file("a")]),
			message("system", [{ type: "hook-notice", text: "Formatter ran." }]),
		]);

		expect(pills(rows)).toEqual([["message:2", ["a"]]]);
	});

	it("shows no pill for the newest turn while it runs", () => {
		const { rows } = group(
			[
				user("Build the report"),
				message("assistant", [text("Done:"), file("a")]),
				user("Another one"),
				message("assistant", [
					text("Working on it:"),
					call("attach", undefined, "attach_file"),
				]),
				message("assistant", [file("b")]),
			],
			{ isTurnActive: true },
		);

		expect(
			pills(rows, { hidesAttachmentRows: true, isChatActive: true }),
		).toEqual([["message:2", ["a"]]]);
	});

	it("leaves an answered turn out while the chat keeps running it, unless it compacts", () => {
		// A stop hook can nudge the agent past its answer within the turn.
		const { rows } = group([
			user("Build the report"),
			message("assistant", [text("Here:"), file("a")]),
		]);

		expect(pills(rows, { isChatActive: true })).toEqual([]);
		expect(pills(rows, { isChatActive: true, isCompacting: true })).toEqual([
			["message:2", ["a"]],
		]);
	});

	it("keeps an answered turn's pill after a compaction until the agent answers again", () => {
		const mockCompactedTurnMessages = [
			user("Build the report"),
			message("assistant", [text("Here:"), file("a")]),
			message("assistant", [call("compact", undefined, "chat_summarized")]),
			message("tool", [
				result("compact", undefined, { name: "chat_summarized" }),
			]),
		];
		const mockRunningChat = { isChatActive: true };

		expect(
			pills(group(mockCompactedTurnMessages).rows, mockRunningChat),
		).toEqual([["message:2", ["a"]]]);
		expect(
			pills(
				group([
					...mockCompactedTurnMessages,
					message("assistant", [text("Rechecked: all good.")]),
				]).rows,
				mockRunningChat,
			),
		).toEqual([]);
	});

	it("leaves an unanswered turn open while it compacts", () => {
		const { rows } = group([
			user("Build the report"),
			message("assistant", [
				text("Working on it:"),
				call("attach", undefined, "attach_file"),
			]),
			message("assistant", [file("a")]),
		]);

		expect(pills(rows, { isChatActive: true, isCompacting: true })).toEqual([]);
	});
});
