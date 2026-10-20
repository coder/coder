import type * as TypesGen from "#/api/typesGenerated";
import { getVisibleContent, isProviderToolResult } from "./messageHelpers";
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
	pieceRange?: { start: number; end: number };
	/**
	 * The row key, plus an ordinal counted separately over the row's work
	 * slices and its output runs. The first of each keeps the bare key, so a
	 * row that splits does not remount its reasoning or streaming text.
	 */
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
	/**
	 * Each member row's work piece, oldest first, or an empty slice for the
	 * stream before a step.
	 */
	slices: Required<RowSlice>[];
	/**
	 * Persisted member message IDs, oldest first. A merged read_file row
	 * contributes every message it merged, so unlike row keys the list grows
	 * at the front when history is prepended into that row.
	 */
	memberIds: number[];
	/**
	 * Durable `${messageId}:${workOrdinal}` keys of the persisted slices,
	 * which survive paging and the block's own completion.
	 */
	pieceKeys: string[];
	/** The work ordinal of the block's slice of the live row. */
	liveWorkOrdinal?: number;
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
type TimelineItem =
	| { type: "slice"; slice: RowSlice }
	| { type: "block"; block: WorkingBlock };

/** A row outside every working block, which renders whole. */
export const wholeRowItem = (
	row: TimelineRow,
	rowIndex: number,
): TimelineItem => ({ type: "slice", slice: { rowIndex, key: row.key } });

export type WorkingBlockLayout = {
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
 * Work (reasoning, tool calls, citations, attached files) folds into a working
 * block. Output (text) renders in the transcript.
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
 * The text item a block continues: the output's last text, when no search
 * followed it and only citations sit between it and the block.
 */
const textBefore = (piece: PieceDraft | undefined, previousIndex: number) => {
	if (piece?.kind !== "output") {
		return undefined;
	}
	const item = piece.items[piece.items.length - 1];
	const { block } = item;
	return block.type === "response" &&
		!block.beforeProviderTool &&
		item.indices[item.indices.length - 1] === previousIndex
		? { piece, item, block }
		: undefined;
};

/** Work and output pieces alternate. */
export const splitRowPieces = (
	blocks: readonly RenderBlock[],
	tools: readonly MergedTool[],
): RowPieces => {
	const visible = new Set(getVisibleContent(blocks, tools).visibleBlocks);
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
		const priorText = textBefore(pieces.at(-1), previousIndex);
		if (block.type !== "sources") {
			previousIndex = index;
		}
		if (!visible.has(block)) {
			continue;
		}

		if (block.type === "sources" && priorText) {
			// OpenAI streams citations between the deltas of one text part, so
			// they fold ahead of the text, where persisted rows store them,
			// instead of splitting it.
			priorText.piece.items.pop();
			if (priorText.piece.items.length === 0) {
				pieces.pop();
			}
			push("work", { block, indices: [index] });
			push("output", priorText.item);
		} else if (
			block.type === "sources" ||
			block.type === "thinking" ||
			block.type === "tool" ||
			// A file belongs to the step that attached it.
			block.type === "file"
		) {
			push("work", { block, indices: [index] });
		} else if (
			block.type === "response" &&
			priorText &&
			// Parsing keeps narration and answer text apart, so merging does too.
			Boolean(priorText.block.narration) === Boolean(block.narration)
		) {
			priorText.item.block = {
				...block,
				text: `${priorText.block.text}${block.text}`,
			};
			priorText.item.indices.push(index);
		} else {
			push("output", { block, indices: [index] });
		}
	}

	const pieceOfBlock = blocks.map(() => -1);
	for (const [pieceIndex, { items }] of pieces.entries()) {
		for (const { indices } of items) {
			for (const index of indices) {
				pieceOfBlock[index] = pieceIndex;
			}
		}
	}
	return {
		pieces: pieces.map(({ kind, items }) => ({
			kind,
			blocks: items.map(({ block }) => block),
		})),
		pieceOfBlock,
	};
};

type GetSliceContentOptions = {
	pieceRange: RowSlice["pieceRange"];
	hideAttachments: boolean;
};

/**
 * The blocks and tools a slice renders: its pieces, or the whole row. Files
 * that hideAttachments drops from agent rows are listed under Attachments once
 * their step persists.
 */
export const getSliceContent = (
	blocks: readonly RenderBlock[],
	tools: readonly MergedTool[],
	{ pieceRange, hideAttachments }: GetSliceContentOptions,
) => {
	const shownBlocks = (pieceBlocks: readonly RenderBlock[]) =>
		hideAttachments
			? pieceBlocks.filter((block) => block.type !== "file")
			: pieceBlocks;
	if (!pieceRange) {
		return {
			blocks: shownBlocks(blocks),
			tools,
			endsRow: true,
			coversRow: true,
		};
	}

	const { pieces } = splitRowPieces(blocks, tools);
	const sliceBlocks = pieces
		.slice(pieceRange.start, pieceRange.end)
		.flatMap((piece) => shownBlocks(piece.blocks));
	return {
		blocks: sliceBlocks,
		// The row's other tools would render again as block-less tools.
		tools: tools.filter((tool) =>
			sliceBlocks.some(
				(block) => block.type === "tool" && block.id === tool.id,
			),
		),
		endsRow: pieceRange.end === pieces.length,
		coversRow: pieceRange.start === 0 && pieceRange.end === pieces.length,
	};
};

/** True for an agent message row left empty once its attachments are hidden. */
export const isAgentAttachmentOnlyRow = (row: TimelineRow): boolean => {
	if (row.type !== "message") {
		return false;
	}
	const { message, parsed } = row.entry;
	if (message.role !== "assistant" || parsed.hookNotices.length > 0) {
		return false;
	}
	const { visibleBlocks } = getVisibleContent(parsed.blocks, parsed.tools);
	return (
		visibleBlocks.length > 0 &&
		visibleBlocks.every((block) => block.type === "file")
	);
};

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
		return row.type === "live" ? splitRowPieces(blocks, tools) : undefined;
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

	return splitRowPieces(blocks, tools);
};

/**
 * Part timestamps are the only reliable clock: message created_at is shared
 * across an insert batch, so it marks when a step was persisted, not when its
 * work started.
 */
const getPartTimestamps = (part: TypesGen.ChatMessagePart): number[] => {
	let timestamps: (string | undefined)[] = [];
	if (part.type === "reasoning") {
		timestamps = [part.created_at, part.completed_at];
	} else if (part.type === "tool-call" || part.type === "tool-result") {
		timestamps = [part.created_at];
	}
	return timestamps
		.filter((timestamp) => timestamp !== undefined)
		.map((timestamp) => Date.parse(timestamp));
};

const rowMessageIds = (row: TimelineRow): readonly number[] =>
	row.type === "live" ? [] : (row.entry.mergedFrom ?? [row.entry.message.id]);

type DraftSlice = {
	rowIndex: number;
	start: number;
	end: number;
	workOrdinal: number;
};

type Draft = {
	slices: DraftSlice[];
	toolIds: Set<string>;
	sourceBlocks: number;
	searches: number;
	times: number[];
	anchorKey?: string;
	ordinal: number;
	containsLiveRow: boolean;
	/** Type of the row whose text, or non-folding content, closed the block. */
	closedByRowType?: TimelineRow["type"];
	isPartial: boolean;
};

/**
 * Any text, even a sentence between two tool calls of one step, closes the
 * open block. Timestamps come from the raw entries, because the timeline rows
 * already dropped tool-result messages and merged read_file runs.
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
	// Until assistant text or a row that never folds loads, the loaded rows may
	// continue a block that older history opened.
	let mayContinueOlderBlock = options.hasMoreMessages;
	let anchorKey: string | undefined;
	let ordinal = 0;
	const closeAnyBlock = (row: TimelineRow) => {
		if (open) {
			open.closedByRowType = row.type;
			open = undefined;
		}
		mayContinueOlderBlock = false;
	};

	for (const [rowIndex, row] of rows.entries()) {
		const split = rowPieces[rowIndex];
		openAtStart[rowIndex] = open;
		if (!split) {
			closeAnyBlock(row);
			if (row.type === "message" && row.entry.message.role !== "assistant") {
				anchorKey = row.key;
				ordinal = 0;
			}
			continue;
		}

		// The empty stream before each step extends a block but never starts
		// one, so a turn's first moments keep the thinking indicator.
		if (split.pieces.length === 0 && open) {
			open.slices.push({ rowIndex, start: 0, end: 0, workOrdinal: 0 });
			open.containsLiveRow = true;
		}

		let workOrdinal = 0;
		for (const [pieceIndex, piece] of split.pieces.entries()) {
			if (piece.kind === "output") {
				closeAnyBlock(row);
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
					isPartial: mayContinueOlderBlock,
				};
				ordinal += 1;
				drafts.push(open);
			}

			open.slices.push({
				rowIndex,
				start: pieceIndex,
				end: pieceIndex + 1,
				workOrdinal,
			});
			workOrdinal += 1;
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

	// Work that rendered nothing joins the block of the nearest visible block
	// before it in its row if that is work, else of the nearest one after it.
	// Failing both, work first in its row joins the block the row continues.
	const getDraftAtBlock = (
		rowIndex: number,
		blockIndex: number,
	): Draft | undefined => {
		const pieceOfBlock = rowPieces[rowIndex]?.pieceOfBlock;
		if (!pieceOfBlock) {
			return undefined;
		}
		const draftAt = (index: number) =>
			pieceDrafts[rowIndex][pieceOfBlock[index]];
		const nextBlockIndex = pieceOfBlock.findIndex(
			(pieceIndex, index) => index > blockIndex && pieceIndex !== -1,
		);
		const draftAfter =
			nextBlockIndex === -1 ? undefined : draftAt(nextBlockIndex);
		const atOrBeforeIndex = pieceOfBlock.findLastIndex(
			(pieceIndex, index) => index <= blockIndex && pieceIndex !== -1,
		);
		if (atOrBeforeIndex === -1) {
			return draftAfter ?? openAtStart[rowIndex];
		}
		return draftAt(atOrBeforeIndex) ?? draftAfter;
	};

	const rowOfMessage = new Map<number, number>();
	for (const [rowIndex, row] of rows.entries()) {
		for (const id of rowMessageIds(row)) {
			rowOfMessage.set(id, rowIndex);
		}
	}

	let previousRowIndex: number | undefined;
	for (const entry of entries) {
		const rowIndex = rowOfMessage.get(entry.message.id);
		const parts = entry.message.content ?? [];
		let getDraft: (partIndex: number) => Draft | undefined;
		if (rowIndex !== undefined) {
			previousRowIndex = rowIndex;
			// A row of only work is one piece, so every part joins its block. A
			// merged read_file row relies on this: its blocks come from several
			// messages, so part positions do not apply.
			getDraft = rowPieces[rowIndex]?.pieces.every(
				(piece) => piece.kind === "work",
			)
				? () => pieceDrafts[rowIndex][0]
				: (partIndex) =>
						getDraftAtBlock(rowIndex, entry.parsed.partBlockIndices[partIndex]);
		} else {
			// A hidden entry, like a tool result, counts toward the block open
			// after the previous row. With none open, a search that rendered no
			// row counts toward the block the next row starts.
			let draft =
				previousRowIndex === undefined
					? undefined
					: openAfter[previousRowIndex];
			const nextRowIndex =
				previousRowIndex === undefined ? 0 : previousRowIndex + 1;
			if (
				parts.some(isProviderToolResult) &&
				rowPieces[nextRowIndex]?.pieces.at(0)?.kind === "work"
			) {
				draft = pieceDrafts[nextRowIndex][0];
			}
			getDraft = () => draft;
		}

		for (const [partIndex, part] of parts.entries()) {
			const timestamps = getPartTimestamps(part);
			// Results, not calls: a step can end with a provider call
			// unanswered, and the next step issues that call again.
			const isSearch = isProviderToolResult(part);
			if (timestamps.length === 0 && !isSearch) {
				continue;
			}

			// A tool's parts join its call's block, even from a later message.
			const toolId =
				part.type === "tool-call" || part.type === "tool-result"
					? part.tool_call_id
					: undefined;
			const draft =
				(toolId === undefined ? undefined : draftOfTool.get(toolId)) ??
				getDraft(partIndex);
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

		// The stream's clock starts at its first reasoning or tool call, a
		// search included, so it belongs to the first work. Before any work it
		// belongs to the block the stream extends, unless a search follows text.
		const firstWorkIndex = liveSplit.pieces.findIndex(
			(piece) => piece.kind === "work",
		);
		const searchLeadInIndex = options.liveBlocks.findLastIndex(
			(block) => block.type === "response" && block.beforeProviderTool,
		);
		let clockDraft: Draft | undefined;
		if (firstWorkIndex !== -1) {
			clockDraft = liveDrafts[firstWorkIndex];
		} else if (searchLeadInIndex === -1) {
			clockDraft = openAtStart[liveRowIndex];
		}
		const streamStartedAt = options.streamState?.startedAt;
		if (clockDraft && streamStartedAt !== undefined) {
			clockDraft.times.push(Date.parse(streamStartedAt));
		}

		// Until the step persists, one flag stands in for its searches. The
		// search sits with its citations, or else after the text it marked.
		if (options.streamState?.providerToolRan) {
			const citedPieceIndex = liveSplit.pieces.findIndex(
				(piece) =>
					piece.kind === "work" &&
					piece.blocks.some((block) => block.type === "sources"),
			);
			const searchDraft =
				citedPieceIndex === -1
					? getDraftAtBlock(liveRowIndex, searchLeadInIndex)
					: liveDrafts[citedPieceIndex];
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
	// call arrives, and unfolds only when text follows it without steps.
	const blockDrafts = drafts.filter(
		(draft) =>
			getStepCount(draft) > 0 ||
			(draft.containsLiveRow &&
				options.isTurnActive &&
				draft.closedByRowType === undefined),
	);

	const sliceKey = (rowIndex: number, sliceOrdinal: number) =>
		sliceOrdinal === 0
			? rows[rowIndex].key
			: `${rows[rowIndex].key}:${sliceOrdinal}`;
	const blockOfPiece: (WorkingBlock | undefined)[][] = rows.map(() => []);
	const rowInBlock = rows.map(() => false);

	const blocks = blockDrafts.map((draft, draftIndex): WorkingBlock => {
		const last = draft.slices[draft.slices.length - 1];
		// A persisted row closing a block completes it even while the chat
		// still runs; text still streaming does not.
		const isLive =
			options.isWorking &&
			draftIndex === blockDrafts.length - 1 &&
			draft.closedByRowType !== "message";
		const liveKey = `working:live:${draft.anchorKey ?? "head"}:${draft.ordinal}`;

		const block: WorkingBlock = {
			key: isLive
				? liveKey
				: `working:through:${sliceKey(last.rowIndex, last.workOrdinal)}`,
			liveKey,
			slices: draft.slices.map(({ rowIndex, start, end, workOrdinal }) => ({
				rowIndex,
				pieceRange: { start, end },
				key: sliceKey(rowIndex, workOrdinal),
			})),
			memberIds: draft.slices.flatMap(({ rowIndex }) =>
				rowMessageIds(rows[rowIndex]),
			),
			pieceKeys: draft.slices.flatMap(({ rowIndex, workOrdinal }) =>
				rowMessageIds(rows[rowIndex]).map((id) => `${id}:${workOrdinal}`),
			),
			liveWorkOrdinal: draft.slices.find(
				({ rowIndex }) => rowIndex === liveRowIndex,
			)?.workOrdinal,
			stepCount: getStepCount(draft),
			isLive,
			isPartial: draft.isPartial,
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
		const split = rowPieces[rowIndex];
		if (!rowInBlock[rowIndex] || !split) {
			items.push(wholeRowItem(row, rowIndex));
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
					pieceRange: { start: runStart, end },
					key: sliceKey(rowIndex, runOrdinal),
				},
			});
			runStart = undefined;
			runOrdinal += 1;
		};

		const pieceCount = split.pieces.length;
		for (let pieceIndex = 0; pieceIndex < pieceCount; pieceIndex++) {
			const block = blockOfPiece[rowIndex][pieceIndex];
			if (!block) {
				runStart ??= pieceIndex;
				continue;
			}

			endRun(pieceIndex);
			const first = block.slices[0];
			if (
				first.rowIndex === rowIndex &&
				first.pieceRange.start === pieceIndex
			) {
				items.push({ type: "block", block });
			}
		}
		endRun(pieceCount);
	}

	return { blocks, items };
};
