import type * as TypesGen from "#/api/typesGenerated";
import { getVisibleContent, isProviderToolResult } from "./messageHelpers";
import { getPartBlockIndices } from "./messageParsing";
import type { TimelineRow } from "./timelineRows";
import type {
	MergedTool,
	ParsedMessageEntry,
	RenderBlock,
	StreamState,
} from "./types";

/** Part of a row: the pieces it renders, or the whole row when absent. */
export type RowSlice = {
	rowIndex: number;
	/** Half-open range of the row's pieces from `splitRowPieces`. */
	pieces?: { start: number; end: number };
	/** Unique among the slices of one row, and the row key for its first. */
	key: string;
};

export type WorkingBlock = {
	/** Follows the newest row, so prepending older history never changes it. */
	key: string;
	/**
	 * Unchanged across the live-to-complete handoff, so expansion recorded
	 * while live survives.
	 */
	liveKey: string;
	/** One work piece per member row, oldest first. */
	slices: RowSlice[];
	/**
	 * Persisted member message IDs, oldest first. A merged read_file row
	 * contributes every message it merged, so unlike row keys the list grows
	 * at the front when history is prepended into that row.
	 */
	memberIds: number[];
	/**
	 * Durable `${messageId}:${workPiece}` keys of the persisted slices, which
	 * survive paging and the block's own completion.
	 */
	pieceKeys: string[];
	/** The work piece ordinal of the block's slice of the live row. */
	livePiece?: number;
	/** Distinct visible tools plus web searches, not rows. */
	stepCount: number;
	isLive: boolean;
	/** Unloaded older history may hold earlier rows of this block. */
	isPartial: boolean;
	/** Epoch ms. */
	startedAt?: number;
	endedAt?: number;
};

/** The timeline in render order: rows, parts of rows, and working blocks. */
export type TimelineItem =
	| { type: "slice"; slice: RowSlice }
	| { type: "block"; block: WorkingBlock };

type WorkingBlockLayout = {
	blocks: WorkingBlock[];
	items: TimelineItem[];
};

export type GroupWorkingBlocksOptions = {
	hasMoreMessages: boolean;
	/** The agent still owns the turn, including while an interrupt drains it. */
	isTurnActive: boolean;
	/** The agent is producing output, so the newest block's clock runs. */
	isWorking: boolean;
	/** False while retry, reconnect, or interrupt callouts render in the row. */
	isLiveRowCollapsible: boolean;
	liveBlocks: readonly RenderBlock[];
	liveTools: readonly MergedTool[];
	streamState?: StreamState | null;
};

/** Tools that need the user's attention or record a transcript boundary. */
const UNCOLLAPSIBLE_TOOLS: ReadonlySet<string> = new Set([
	"ask_user_question",
	"propose_plan",
	"chat_summarized",
	"chat_cleared",
]);

/**
 * A run of a row's visible blocks. Work, the reasoning, tool calls, and
 * citations, folds into a working block. Output, the text and files the
 * assistant addresses to the user, renders in the transcript and closes the
 * block before it.
 */
type RowPiece = {
	kind: "work" | "output";
	blocks: RenderBlock[];
};

type PieceItem = { block: RenderBlock; indices: number[] };
type PieceDraft = { kind: RowPiece["kind"]; items: PieceItem[] };

type RowPieces = {
	pieces: RowPiece[];
	/** Each block's piece, or -1 for a block that renders nothing. */
	pieceOfBlock: number[];
};

/**
 * The text item a block continues: the output's last text, with nothing but
 * citations between it and the block.
 */
const textBefore = (piece: PieceDraft | undefined, previousIndex: number) => {
	const item = piece?.kind === "output" ? piece.items.at(-1) : undefined;
	const block = item?.block;
	return item &&
		block?.type === "response" &&
		!block.beforeProviderTool &&
		item.indices.at(-1) === previousIndex
		? { item, block }
		: undefined;
};

const splitVisibleBlocks = (
	blocks: readonly RenderBlock[],
	visible: ReadonlySet<RenderBlock>,
): RowPieces => {
	const pieces: PieceDraft[] = [];
	const push = (kind: RowPiece["kind"], item: PieceItem) => {
		const last = pieces.at(-1);
		if (last?.kind === kind) {
			last.items.push(item);
		} else {
			pieces.push({ kind, items: [item] });
		}
	};

	// Hidden blocks still separate text, so adjacency uses every block but
	// citations.
	let previousIndex = -1;
	for (const [index, block] of blocks.entries()) {
		const text = textBefore(pieces.at(-1), previousIndex);
		if (block.type !== "sources") {
			previousIndex = index;
		}
		if (!visible.has(block)) {
			continue;
		}

		if (block.type === "sources") {
			// OpenAI streams citations between the deltas of one text part, so
			// they fold ahead of the text instead of splitting it.
			const output = pieces.at(-1);
			if (text && output) {
				output.items.pop();
				if (output.items.length === 0) {
					pieces.pop();
				}
				push("work", { block, indices: [index] });
				push("output", text.item);
			} else {
				push("work", { block, indices: [index] });
			}
		} else if (block.type === "thinking" || block.type === "tool") {
			push("work", { block, indices: [index] });
		} else if (
			block.type === "response" &&
			text &&
			(text.block.narration ?? false) === (block.narration ?? false)
		) {
			text.item.block = { ...block, text: `${text.block.text}${block.text}` };
			text.item.indices.push(index);
		} else {
			push("output", { block, indices: [index] });
		}
	}

	const pieceOfBlock = blocks.map(() => -1);
	return {
		pieces: pieces.map((piece, pieceIndex) => {
			for (const item of piece.items) {
				for (const index of item.indices) {
					pieceOfBlock[index] = pieceIndex;
				}
			}
			return {
				kind: piece.kind,
				blocks: piece.items.map(({ block }) => block),
			};
		}),
		pieceOfBlock,
	};
};

/**
 * Splits a row's visible blocks into alternating work and output pieces.
 * Citations right after text fold ahead of it, where persisted rows store
 * them.
 */
export const splitRowPieces = (
	blocks: readonly RenderBlock[],
	tools: readonly MergedTool[],
): RowPiece[] =>
	splitVisibleBlocks(
		blocks,
		new Set(getVisibleContent(blocks, tools).visibleBlocks),
	).pieces;

/**
 * Pieces of a row that can fold, or undefined for a row that never folds and
 * closes any open block. A live row that shows nothing yet has no pieces: it
 * is the turn working on its next step.
 */
const getRowPieces = (
	row: TimelineRow,
	options: GroupWorkingBlocksOptions,
): RowPieces | undefined => {
	let blocks: readonly RenderBlock[];
	let tools: readonly MergedTool[];
	if (row.type === "live") {
		if (!options.isLiveRowCollapsible) {
			return undefined;
		}
		blocks = options.liveBlocks;
		tools = options.liveTools;
	} else {
		const { message, parsed } = row.entry;
		if (message.role !== "assistant" || parsed.hookNotices.length > 0) {
			return undefined;
		}
		blocks = parsed.blocks;
		tools = parsed.tools;
	}

	const { visibleBlocks, visibleTools } = getVisibleContent(blocks, tools);
	if (visibleBlocks.length === 0) {
		return row.type === "live"
			? { pieces: [], pieceOfBlock: blocks.map(() => -1) }
			: undefined;
	}

	if (visibleTools.some((tool) => UNCOLLAPSIBLE_TOOLS.has(tool.name))) {
		return undefined;
	}

	// A tool still running while the turn is parked (requires_action) is
	// waiting on a client, so it stays visible like a question.
	if (
		!options.isTurnActive &&
		visibleTools.some((tool) => tool.status === "running")
	) {
		return undefined;
	}

	return splitVisibleBlocks(blocks, new Set(visibleBlocks));
};

/**
 * Part timestamps are the only reliable clock: message created_at is shared
 * across an insert batch, so it marks when a step was persisted, not when its
 * work started.
 */
const getPartTimestamps = (part: TypesGen.ChatMessagePart): number[] => {
	const timestamps =
		part.type === "reasoning"
			? [part.created_at, part.completed_at]
			: part.type === "tool-call" || part.type === "tool-result"
				? [part.created_at]
				: [];
	return timestamps
		.filter((timestamp) => timestamp !== undefined)
		.map((timestamp) => Date.parse(timestamp));
};

const rowMessageIds = (row: TimelineRow): readonly number[] =>
	row.type === "live" ? [] : (row.entry.mergedFrom ?? [row.entry.message.id]);

type Draft = {
	slices: { rowIndex: number; start: number; end: number }[];
	toolIds: Set<string>;
	sourceBlocks: number;
	searches: number;
	times: number[];
	anchorKey?: string;
	ordinal: number;
	containsLiveRow: boolean;
	followedByOutput: boolean;
};

/**
 * Folds consecutive work into working blocks. Any output, even a sentence
 * between two tool calls of one step, closes the open block, and the next
 * work opens a new one below it. Timestamps come from the raw entries,
 * because the timeline rows already dropped tool-result messages and merged
 * read_file runs.
 */
export const groupWorkingBlocks = (
	rows: readonly TimelineRow[],
	entries: readonly ParsedMessageEntry[],
	options: GroupWorkingBlocksOptions,
): WorkingBlockLayout => {
	const rowPieces = rows.map((row) => getRowPieces(row, options));
	const drafts: Draft[] = [];
	const pieceDrafts: (Draft | undefined)[][] = rows.map(() => []);
	const openAtStart: (Draft | undefined)[] = [];
	const openAfter: (Draft | undefined)[] = [];
	const draftOfTool = new Map<string, Draft>();
	let open: Draft | undefined;
	let anchorKey: string | undefined;
	let ordinal = 0;

	for (const [rowIndex, row] of rows.entries()) {
		const split = rowPieces[rowIndex];
		openAtStart[rowIndex] = open;
		if (!split) {
			open = undefined;
			if (row.type === "message" && row.entry.message.role !== "assistant") {
				anchorKey = row.key;
				ordinal = 0;
			}
			continue;
		}

		// The empty stream before each step extends a block but never starts
		// one, so a turn's first moments keep the thinking indicator.
		if (split.pieces.length === 0 && open) {
			open.slices.push({ rowIndex, start: 0, end: 0 });
			open.containsLiveRow = true;
		}

		for (const [pieceIndex, piece] of split.pieces.entries()) {
			if (piece.kind === "output") {
				if (open) {
					open.followedByOutput = true;
				}
				open = undefined;
				continue;
			}

			if (!open) {
				open = {
					slices: [],
					toolIds: new Set(),
					sourceBlocks: 0,
					searches: 0,
					times: [],
					anchorKey,
					ordinal,
					containsLiveRow: false,
					followedByOutput: false,
				};
				ordinal += 1;
				drafts.push(open);
			}

			open.slices.push({ rowIndex, start: pieceIndex, end: pieceIndex + 1 });
			open.containsLiveRow ||= row.type === "live";
			pieceDrafts[rowIndex][pieceIndex] = open;
			for (const block of piece.blocks) {
				if (block.type === "tool") {
					open.toolIds.add(block.id);
					draftOfTool.set(block.id, open);
				} else if (block.type === "sources") {
					open.sourceBlocks += 1;
				}
			}
		}
		openAfter[rowIndex] = open;
	}

	// Work that rendered nothing, like a search without citations, counts
	// toward the block of the nearest visible work before it in its row, or
	// right after the output before it. Before the row's first block it
	// counts toward the block that row continues.
	const getSlotDraft = (
		rowIndex: number,
		blockIndex: number,
	): Draft | undefined => {
		const split = rowPieces[rowIndex];
		if (!split) {
			return undefined;
		}
		const { pieces, pieceOfBlock } = split;
		const draftAt = (index: number) => {
			const piece = pieceOfBlock[index];
			return pieces[piece].kind === "work"
				? pieceDrafts[rowIndex][piece]
				: undefined;
		};
		const next = pieceOfBlock.findIndex(
			(piece, index) => index > blockIndex && piece !== -1,
		);
		const draftAfter = next === -1 ? undefined : draftAt(next);
		const previous = pieceOfBlock.findLastIndex(
			(piece, index) => index <= blockIndex && piece !== -1,
		);
		if (previous === -1) {
			return draftAfter ?? openAtStart[rowIndex];
		}
		return draftAt(previous) ?? draftAfter;
	};

	const getRowDraft = (
		rowIndex: number,
		entry: ParsedMessageEntry,
	): ((partIndex: number) => Draft | undefined) => {
		// A merged read_file row is all work, and its blocks come from
		// several messages, so part positions do not apply.
		if (rowPieces[rowIndex]?.pieces.every((piece) => piece.kind === "work")) {
			return () => pieceDrafts[rowIndex][0];
		}

		let partBlockIndices: number[] | undefined;
		return (partIndex) => {
			partBlockIndices ??= getPartBlockIndices(entry.message.content);
			return getSlotDraft(rowIndex, partBlockIndices[partIndex]);
		};
	};

	const rowOfMessage = new Map<number, number>();
	for (const [rowIndex, row] of rows.entries()) {
		for (const id of rowMessageIds(row)) {
			rowOfMessage.set(id, rowIndex);
		}
	}

	let previousRow: number | undefined;
	for (const entry of entries) {
		const rowIndex = rowOfMessage.get(entry.message.id);
		const parts = entry.message.content ?? [];
		let getDraft: (partIndex: number) => Draft | undefined;
		if (rowIndex !== undefined) {
			previousRow = rowIndex;
			getDraft = getRowDraft(rowIndex, entry);
		} else {
			// A hidden entry, like a tool result, counts toward the block open
			// after the previous row. A search that rendered no row counts
			// toward the block the next row starts.
			const nextRow = previousRow === undefined ? 0 : previousRow + 1;
			const draft =
				(previousRow === undefined ? undefined : openAfter[previousRow]) ??
				(parts.some(isProviderToolResult) &&
				rowPieces[nextRow]?.pieces[0]?.kind === "work"
					? pieceDrafts[nextRow][0]
					: undefined);
			getDraft = () => draft;
		}

		for (const [partIndex, part] of parts.entries()) {
			const timestamps = getPartTimestamps(part);
			const isSearch = isProviderToolResult(part);
			if (timestamps.length === 0 && !isSearch) {
				continue;
			}

			const toolDraft =
				(part.type === "tool-call" || part.type === "tool-result") &&
				!part.provider_executed
					? draftOfTool.get(part.tool_call_id || `${part.type}-${partIndex}`)
					: undefined;
			const draft = toolDraft ?? getDraft(partIndex);
			if (!draft) {
				continue;
			}
			draft.times.push(...timestamps);
			if (isSearch) {
				draft.searches += 1;
			}
		}
	}

	const liveRowIndex = rows.findIndex((row) => row.type === "live");
	const liveSplit = rowPieces[liveRowIndex];
	if (liveSplit) {
		const liveDrafts = pieceDrafts[liveRowIndex];
		for (const tool of options.liveTools) {
			const draft = draftOfTool.get(tool.id);
			if (draft && tool.startedAt !== undefined) {
				draft.times.push(Date.parse(tool.startedAt));
			}
		}

		// The stream's clock starts at its first reasoning or tool call, so it
		// belongs to the stream's first work. Before any work it belongs to
		// the block the stream extends, unless a search follows text.
		const firstWork = liveSplit.pieces.findIndex(
			(piece) => piece.kind === "work",
		);
		const marksSearch = options.liveBlocks.some(
			(block) => block.type === "response" && block.beforeProviderTool,
		);
		const clockDraft =
			firstWork === -1
				? marksSearch
					? undefined
					: openAtStart[liveRowIndex]
				: liveDrafts[firstWork];
		const streamStartedAt = options.streamState?.startedAt;
		if (clockDraft && streamStartedAt !== undefined) {
			clockDraft.times.push(Date.parse(streamStartedAt));
		}

		// Until the step persists, one flag stands in for its searches. The
		// search sits with its citations, or else after the text it marked.
		if (options.streamState?.providerToolRan) {
			const cited = liveSplit.pieces.findIndex(
				(piece) =>
					piece.kind === "work" &&
					piece.blocks.some((block) => block.type === "sources"),
			);
			const searchDraft =
				cited === -1
					? getSlotDraft(
							liveRowIndex,
							options.liveBlocks.findLastIndex(
								(block) =>
									block.type === "response" && block.beforeProviderTool,
							),
						)
					: liveDrafts[cited];
			if (searchDraft) {
				searchDraft.searches += 1;
			}
		}
	}

	// Source blocks count as searches only when no provider result does:
	// some providers cite without a search, and an interrupt commits a
	// search's result apart from the answer citing it.
	const getStepCount = (draft: Draft) =>
		draft.toolIds.size + (draft.searches || draft.sourceBlocks);

	// Completed reasoning alone stays visible. The live turn folds from its
	// first reasoning, so thinking never shows and then vanishes once a tool
	// call arrives, and unfolds only when output follows it without steps.
	const kept = drafts.filter(
		(draft) =>
			getStepCount(draft) > 0 ||
			(draft.containsLiveRow &&
				options.isTurnActive &&
				!draft.followedByOutput),
	);

	const workOrdinal = (rowIndex: number, pieceIndex: number) =>
		(rowPieces[rowIndex]?.pieces ?? [])
			.slice(0, pieceIndex)
			.filter((piece) => piece.kind === "work").length;
	const sliceKey = (rowIndex: number, ordinal: number) =>
		ordinal === 0 ? rows[rowIndex].key : `${rows[rowIndex].key}:${ordinal}`;
	const lastMessageRowIndex = rows.findLastIndex(
		(row) => row.type === "message",
	);
	const lastDraft = kept.at(-1);
	const blockOfPiece: (WorkingBlock | undefined)[][] = rows.map(() => []);
	const rowInBlock = rows.map(() => false);

	const blocks = kept.map((draft): WorkingBlock => {
		const first = draft.slices[0];
		const last = draft.slices[draft.slices.length - 1];
		const lastOrdinal = workOrdinal(last.rowIndex, last.start);

		// Output persisted after the block completes it even while the chat
		// still runs; output still streaming does not.
		const isLive =
			options.isWorking &&
			draft === lastDraft &&
			(rows[last.rowIndex].type === "live" ||
				(last.rowIndex === lastMessageRowIndex &&
					last.end === rowPieces[last.rowIndex]?.pieces.length));

		const liveKey = `working:live:${draft.anchorKey ?? "head"}:${draft.ordinal}`;
		const throughKey = `working:through:${sliceKey(last.rowIndex, lastOrdinal)}`;

		const block: WorkingBlock = {
			key: isLive ? liveKey : throughKey,
			liveKey,
			slices: draft.slices.map(({ rowIndex, start, end }) => ({
				rowIndex,
				pieces: { start, end },
				key: sliceKey(rowIndex, workOrdinal(rowIndex, start)),
			})),
			memberIds: draft.slices.flatMap(({ rowIndex }) =>
				rowMessageIds(rows[rowIndex]),
			),
			pieceKeys: draft.slices.flatMap(({ rowIndex, start }) =>
				rowMessageIds(rows[rowIndex]).map(
					(id) => `${id}:${workOrdinal(rowIndex, start)}`,
				),
			),
			livePiece: draft.slices
				.filter(({ rowIndex }) => rowIndex === liveRowIndex)
				.map(({ start }) => workOrdinal(liveRowIndex, start))
				.at(0),
			stepCount: getStepCount(draft),
			isLive,
			isPartial:
				options.hasMoreMessages && first.rowIndex === 0 && first.start === 0,
			startedAt: draft.times.length > 0 ? Math.min(...draft.times) : undefined,
			endedAt:
				isLive || draft.times.length === 0
					? undefined
					: Math.max(...draft.times),
		};

		for (const { rowIndex, start } of draft.slices) {
			blockOfPiece[rowIndex][start] = block;
			rowInBlock[rowIndex] = true;
		}
		return block;
	});

	// A row in a block renders its other pieces around the block in order,
	// each run of them as one slice.
	const items: TimelineItem[] = [];
	for (const [rowIndex, row] of rows.entries()) {
		if (!rowInBlock[rowIndex]) {
			items.push({ type: "slice", slice: { rowIndex, key: row.key } });
			continue;
		}

		let runStart: number | undefined;
		let runOrdinal = 0;
		const endRun = (end: number) => {
			if (runStart === undefined) {
				return;
			}
			items.push({
				type: "slice",
				slice: {
					rowIndex,
					pieces: { start: runStart, end },
					key: sliceKey(rowIndex, runOrdinal),
				},
			});
			runStart = undefined;
			runOrdinal += 1;
		};

		const pieceCount = rowPieces[rowIndex]?.pieces.length ?? 0;
		for (let pieceIndex = 0; pieceIndex < pieceCount; pieceIndex++) {
			const block = blockOfPiece[rowIndex][pieceIndex];
			if (!block) {
				runStart ??= pieceIndex;
				continue;
			}

			endRun(pieceIndex);
			const first = block.slices[0];
			if (first.rowIndex === rowIndex && first.pieces?.start === pieceIndex) {
				items.push({ type: "block", block });
			}
		}
		endRun(pieceCount);
	}

	return { blocks, items };
};
