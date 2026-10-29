import type * as TypesGen from "#/api/typesGenerated";
import {
	getMessageFileAttachments,
	getVisibleContent,
	type MessageAttachment,
} from "./messageHelpers";
import { isHiddenProviderExecutedPart } from "./messageParsing";
import type { TimelineMessageRow, TimelineRow } from "./timelineRows";
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
	 * The row key, plus a count of the row's block slices or output pieces
	 * before this one. A count of 0 keeps the bare key, so splitting or
	 * folding a row does not remount its reasoning or text.
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
	/** Set once its turn has ended, so the turn's fold starts collapsed. */
	turnKey?: string;
	/**
	 * Member rows' pieces, oldest first: a slice per work piece, or per row
	 * once the turn ends, and an empty slice for the stream before a step.
	 */
	slices: Required<RowSlice>[];
	/**
	 * Persisted member message IDs, oldest first. A merged read_file row
	 * contributes every message it merged, so unlike row keys the list grows
	 * at the front when history is prepended into that row.
	 */
	memberIds: number[];
	/**
	 * Durable `${messageId}:${workOrdinal}` keys of the persisted work pieces,
	 * which survive paging, the block's completion, and its turn's fold.
	 */
	pieceKeys: string[];
	/** The work ordinal of the block's slice of the live row. */
	liveWorkOrdinal?: number;
	/** Distinct visible tools plus hidden provider calls, not rows. */
	stepCount: number;
	isLive: boolean;
	/** Unloaded older history may hold earlier rows of this block. */
	isPartial: boolean;
	/** Epoch ms. */
	startedAt?: number;
	endedAt?: number;
};

/** The newest piece wins; the live row's key spans turns, so it is left out. */
export const getExpansionKeys = (block: WorkingBlock, itemKey: string) =>
	block.turnKey ? [block.turnKey] : [itemKey, ...block.pieceKeys];

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
	/** A turn parked on a client tool (requires_action) has not ended. */
	isTurnParked: boolean;
	/** The agent is producing output, so the newest block's clock runs. */
	isWorking: boolean;
	/** False while retry, reconnect, or interrupt callouts render in the row. */
	isLiveRowCollapsible: boolean;
	liveBlocks: readonly RenderBlock[];
	liveTools: readonly MergedTool[];
	streamState?: StreamState | null;
	/** Live keys of blocks the reader focused or toggled, kept even without steps. */
	keptBlockKeys?: ReadonlySet<string>;
};

const CONTEXT_BOUNDARY_TOOLS: ReadonlySet<string> = new Set([
	"chat_summarized",
	"chat_cleared",
]);

/** Tools that need the user's attention or record a transcript boundary. */
const UNCOLLAPSIBLE_TOOLS: ReadonlySet<string> = new Set([
	"ask_user_question",
	"propose_plan",
	...CONTEXT_BOUNDARY_TOOLS,
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

const isNarration = (block: RenderBlock | undefined) =>
	block?.type === "response" && Boolean(block.narration);

/**
 * Work and output pieces alternate, except that labeled narration is an
 * output piece of its own.
 */
export const splitRowPieces = (
	blocks: readonly RenderBlock[],
	tools: readonly MergedTool[],
): RowPieces => {
	const visible = new Set(getVisibleContent(blocks, tools).visibleBlocks);
	const pieces: PieceDraft[] = [];
	const push = (kind: RowPiece["kind"], item: PieceItem) => {
		const last = pieces.at(-1);
		if (
			last?.kind === kind &&
			(kind === "work" ||
				isNarration(last.items.at(-1)?.block) === isNarration(item.block))
		) {
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

const isContextBoundaryRow = (row: TimelineMessageRow): boolean =>
	row.entry.parsed.tools.some((tool) => CONTEXT_BOUNDARY_TOOLS.has(tool.name));

const isStepBlock = (block: RenderBlock) =>
	block.type === "thinking" ||
	block.type === "tool" ||
	(block.type === "response" &&
		Boolean(block.beforeProviderTool || block.narration));

/** The row shows text after its last step; attached files are not steps. */
const hasAnswer = (row: TimelineMessageRow): boolean => {
	const { blocks, tools } = row.entry.parsed;
	const { visibleBlocks } = getVisibleContent(blocks, tools);
	return visibleBlocks
		.slice(visibleBlocks.findLastIndex(isStepBlock) + 1)
		.some((block) => block.type === "response");
};

/**
 * Chat state that decides whether the newest turn's Attachments pill, which
 * `TurnAttachments` renders, shows yet and which of the turn's rows hosts it.
 */
export type TurnAttachmentsOptions = {
	/**
	 * Collapsed steps hide attachment-only rows, so those rows never host the
	 * Attachments pill.
	 */
	hidesAttachmentRows: boolean;
	/** The chat may still add to its newest turn. */
	isChatActive: boolean;
	/** A streaming compaction adds no answer or file to a turn. */
	isCompacting: boolean;
};

/** Each turn's assistant rows. A turn starts at each user message. */
const splitIntoTurns = (
	rows: readonly TimelineRow[],
): TimelineMessageRow[][] => {
	const turns: TimelineMessageRow[][] = [[]];
	for (const row of rows) {
		if (row.type !== "message") {
			continue;
		}

		const { role } = row.entry.message;
		if (role === "user") {
			turns.push([]);
		} else if (role === "assistant") {
			turns[turns.length - 1].push(row);
		}
	}
	return turns;
};

/**
 * A context clear (`chat_cleared`) or summary (`chat_summarized`) is not part
 * of the turn's answer, and collapsed steps hide attachment-only rows.
 */
const canHostPill = (
	row: TimelineMessageRow,
	hidesAttachmentRows: boolean,
): boolean => {
	if (isContextBoundaryRow(row)) {
		return false;
	}
	return !hidesAttachmentRows || !isAgentAttachmentOnlyRow(row);
};

/**
 * A running chat can still add files or text to its newest turn, even after
 * an answer. An answer followed by a compaction or context clear is final:
 * chatd resumes only an unanswered turn after a compaction.
 */
const hasNewestTurnEnded = (
	turnRows: readonly TimelineMessageRow[],
	pillRowIndex: number,
	{ isChatActive, isCompacting }: TurnAttachmentsOptions,
): boolean => {
	if (!isChatActive) {
		return true;
	}
	if (!hasAnswer(turnRows[pillRowIndex])) {
		return false;
	}
	return (
		isCompacting || turnRows.slice(pillRowIndex + 1).some(isContextBoundaryRow)
	);
};

/**
 * Each turn's agent files, keyed by the row that shows them in the
 * Attachments pill: the turn's last assistant row that can host it.
 */
export const getTurnAttachmentsByRow = (
	rows: readonly TimelineRow[],
	options: TurnAttachmentsOptions,
): ReadonlyMap<string, readonly MessageAttachment[]> => {
	const turns = splitIntoTurns(rows);
	const attachmentsByRow = new Map<string, readonly MessageAttachment[]>();
	for (const [turnIndex, turnRows] of turns.entries()) {
		const attachments = turnRows.flatMap((row) =>
			getMessageFileAttachments(row.entry.message),
		);
		const pillRowIndex = turnRows.findLastIndex((row) =>
			canHostPill(row, options.hidesAttachmentRows),
		);
		if (attachments.length === 0 || pillRowIndex === -1) {
			continue;
		}

		const isNewestTurn = turnIndex === turns.length - 1;
		if (isNewestTurn && !hasNewestTurnEnded(turnRows, pillRowIndex, options)) {
			continue;
		}

		attachmentsByRow.set(turnRows[pillRowIndex].key, attachments);
	}
	return attachmentsByRow;
};

const sliceEndsRow = (
	row: TimelineRow,
	{ pieceRange }: Required<RowSlice>,
): boolean => {
	if (row.type !== "message") {
		return false;
	}
	const { blocks, tools } = row.entry.parsed;
	return pieceRange.end === splitRowPieces(blocks, tools).pieces.length;
};

/**
 * A pill row that ends inside a collapsed block renders nothing below the
 * block, so the block shows the row's pill.
 */
export const getBlockTurnAttachments = (
	block: WorkingBlock,
	rows: readonly TimelineRow[],
	turnAttachmentsByRow: ReadonlyMap<string, readonly MessageAttachment[]>,
): readonly MessageAttachment[] | undefined => {
	for (const slice of block.slices) {
		const row = rows[slice.rowIndex];
		const attachments = turnAttachmentsByRow.get(row.key);
		if (attachments && sliceEndsRow(row, slice)) {
			return attachments;
		}
	}
	return undefined;
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

/**
 * A provider-executed call that renders no row, so only its result shows
 * that the step ran. Web searches render rows and count as tools.
 */
const isHiddenProviderResult = (part: TypesGen.ChatMessagePart): boolean =>
	part.type === "tool-result" && isHiddenProviderExecutedPart(part);

/** The persisted message IDs a row renders; the live row has none. */
export const rowMessageIds = (row: TimelineRow): readonly number[] =>
	row.type === "live" ? [] : (row.entry.mergedFrom ?? [row.entry.message.id]);

type PiecePosition = { rowIndex: number; pieceIndex: number };

type DraftSlice = {
	rowIndex: number;
	start: number;
	end: number;
	keyOrdinal: number;
	/** Each work piece's ordinal in its row; the empty stream takes 0. */
	workOrdinals: number[];
};

type Draft = {
	slices: DraftSlice[];
	toolIds: Set<string>;
	sourceBlocks: number;
	searches: number;
	hasProviderTool: boolean;
	times: number[];
	anchorKey?: string;
	ordinal: number;
	containsLiveRow: boolean;
	containsNarration: boolean;
	/** Type of the row whose text, or non-folding content, closed the block. */
	closedByRowType?: TimelineRow["type"];
	isPartial: boolean;
};

/**
 * Text closes the open block in an open turn. An ended turn folds its
 * narration and the text before its last work. Timestamps come from the
 * raw entries, as the rows dropped tool results and merged read_file runs.
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
	// The latest turn stays open while the agent owns it or waits on a
	// client; every earlier turn has ended. Hook notices split a turn but do
	// not start one.
	const latestTurnStart = rows.findLastIndex(
		(row) => row.type === "message" && row.entry.message.role === "user",
	);
	const isLatestTurnOpen = options.isTurnActive || options.isTurnParked;
	const isRowEnded = (rowIndex: number) =>
		!isLatestTurnOpen || rowIndex < latestTurnStart;
	const slicesInRow = rows.map(() => 0);
	let open: Draft | undefined;
	// An ended turn's text waits here until later work or narration folds it,
	// or a row that never folds closes the block.
	let pendingText: PiecePosition[] = [];
	// Until a row that never folds, or an open turn's text, loads, the loaded
	// rows may continue a block that older history opened, since an ended
	// turn's text leaves its block open.
	let mayContinueOlderBlock = options.hasMoreMessages;
	let anchorKey: string | undefined;
	let ordinal = 0;
	const closeAnyBlock = (row: TimelineRow) => {
		if (open) {
			open.closedByRowType = row.type;
			open = undefined;
		}
		pendingText = [];
		mayContinueOlderBlock = false;
	};
	const joinOrOpenBlock = ({
		rowIndex,
		pieceIndex,
		workOrdinal,
		isEnded,
	}: PiecePosition & { workOrdinal?: number; isEnded: boolean }) => {
		if (!open) {
			open = {
				slices: [],
				toolIds: new Set(),
				sourceBlocks: 0,
				searches: 0,
				hasProviderTool: false,
				times: [],
				anchorKey,
				ordinal,
				containsLiveRow: false,
				containsNarration: false,
				isPartial: mayContinueOlderBlock,
			};
			ordinal += 1;
			drafts.push(open);
		}

		const workOrdinals = workOrdinal === undefined ? [] : [workOrdinal];
		// An ended turn's row folds as one slice.
		const last = open.slices.at(-1);
		if (isEnded && last?.rowIndex === rowIndex && last.end === pieceIndex) {
			last.end += 1;
			last.workOrdinals.push(...workOrdinals);
		} else {
			open.slices.push({
				rowIndex,
				start: pieceIndex,
				end: pieceIndex + 1,
				keyOrdinal: slicesInRow[rowIndex]++,
				workOrdinals,
			});
		}
		open.containsLiveRow ||= rows[rowIndex].type === "live";
		pieceDrafts[rowIndex][pieceIndex] = open;
		return open;
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

		const isEnded = isRowEnded(rowIndex);
		// The empty stream before each step extends a block but never starts
		// one, so a turn's first moments keep the thinking indicator.
		if (split.pieces.length === 0 && open) {
			open.slices.push({
				rowIndex,
				start: 0,
				end: 0,
				keyOrdinal: 0,
				workOrdinals: [0],
			});
			open.containsLiveRow = true;
		}

		let workOrdinal = 0;
		for (const [pieceIndex, piece] of split.pieces.entries()) {
			const isFoldingNarration = isEnded && piece.blocks.some(isNarration);
			if (piece.kind === "output" && !isFoldingNarration) {
				if (isEnded) {
					pendingText.push({ rowIndex, pieceIndex });
				} else {
					closeAnyBlock(row);
				}
				continue;
			}

			for (const position of pendingText) {
				joinOrOpenBlock({ ...position, isEnded });
			}
			pendingText = [];
			if (piece.kind === "output") {
				joinOrOpenBlock({
					rowIndex,
					pieceIndex,
					isEnded,
				}).containsNarration = true;
				continue;
			}
			const draft = joinOrOpenBlock({
				rowIndex,
				pieceIndex,
				workOrdinal,
				isEnded,
			});
			workOrdinal += 1;
			const tools =
				row.type === "live" ? options.liveTools : row.entry.parsed.tools;
			for (const block of piece.blocks) {
				if (block.type === "tool") {
					draft.toolIds.add(block.id);
					draftOfTool.set(block.id, draft);
					draft.hasProviderTool ||= tools.some(
						(tool) => tool.id === block.id && tool.providerExecuted === true,
					);
				} else if (block.type === "sources") {
					draft.sourceBlocks += 1;
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
			// after the previous row. A hidden provider call counts toward the
			// block the next row starts or continues, when there is one.
			let draft =
				previousRowIndex === undefined
					? undefined
					: openAfter[previousRowIndex];
			const nextRowIndex =
				previousRowIndex === undefined ? 0 : previousRowIndex + 1;
			const nextRowDraft = pieceDrafts[nextRowIndex]?.[0];
			if (parts.some(isHiddenProviderResult) && nextRowDraft) {
				draft = nextRowDraft;
			}
			getDraft = () => draft;
		}

		for (const [partIndex, part] of parts.entries()) {
			const timestamps = getPartTimestamps(part);
			// Results, not calls: a step can end with a provider call
			// unanswered, and the next step issues that call again.
			const isSearch = isHiddenProviderResult(part);
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

	// Source blocks count as searches only when no provider search does,
	// hidden or shown as a tool: some providers cite without a search, and an
	// interrupt commits a search's result apart from the answer citing it.
	const getStepCount = (draft: Draft) =>
		draft.toolIds.size +
		draft.searches +
		(draft.searches > 0 || draft.hasProviderTool ? 0 : draft.sourceBlocks);

	const getLiveKey = (draft: Draft) =>
		`working:live:${draft.anchorKey ?? "head"}:${draft.ordinal}`;

	// Completed reasoning alone stays visible. The live turn folds from its
	// first reasoning, so thinking never shows and then vanishes once a tool
	// call arrives, and unfolds only when text follows it without steps.
	const blockDrafts = drafts.filter(
		(draft) =>
			getStepCount(draft) > 0 ||
			// Otherwise a tool-less ended turn's narration reads as its answer.
			draft.containsNarration ||
			(draft.containsLiveRow &&
				options.isTurnActive &&
				draft.closedByRowType === undefined) ||
			options.keptBlockKeys?.has(getLiveKey(draft)),
	);

	const sliceKey = (rowIndex: number, sliceOrdinal: number) =>
		sliceOrdinal === 0
			? rows[rowIndex].key
			: `${rows[rowIndex].key}:${sliceOrdinal}`;
	const blockOfPiece: (WorkingBlock | undefined)[][] = rows.map(() => []);
	const rowInBlock = rows.map(() => false);

	const blocks = blockDrafts.map((draft, draftIndex): WorkingBlock => {
		const last = draft.slices[draft.slices.length - 1];
		const workPieces = draft.slices.flatMap(({ rowIndex, workOrdinals }) =>
			workOrdinals.map((workOrdinal) => ({ rowIndex, workOrdinal })),
		);
		// Keyed on the last work piece's slice key in an open turn, so an ended
		// turn's fold keeps the key of the turn's last block while it ran. A
		// block of only narration keys on its last row.
		const lastWork = workPieces.at(-1);
		const throughKey = lastWork
			? sliceKey(lastWork.rowIndex, lastWork.workOrdinal)
			: rows[last.rowIndex].key;
		// A persisted row closing a block completes it even while the chat
		// still runs; text still streaming does not.
		const isLive =
			options.isWorking &&
			draftIndex === blockDrafts.length - 1 &&
			draft.closedByRowType !== "message";
		const liveKey = getLiveKey(draft);

		const block: WorkingBlock = {
			key: isLive ? liveKey : `working:through:${throughKey}`,
			liveKey,
			turnKey: isRowEnded(last.rowIndex)
				? `working:turn:${throughKey}`
				: undefined,
			slices: draft.slices.map(({ rowIndex, start, end, keyOrdinal }) => ({
				rowIndex,
				pieceRange: { start, end },
				key: sliceKey(rowIndex, keyOrdinal),
			})),
			memberIds: draft.slices.flatMap(({ rowIndex }) =>
				rowMessageIds(rows[rowIndex]),
			),
			pieceKeys: workPieces.flatMap(({ rowIndex, workOrdinal }) =>
				rowMessageIds(rows[rowIndex]).map((id) => `${id}:${workOrdinal}`),
			),
			liveWorkOrdinal: draft.slices.find(
				({ rowIndex }) => rowIndex === liveRowIndex,
			)?.workOrdinals[0],
			stepCount: getStepCount(draft),
			isLive,
			isPartial: draft.isPartial,
			startedAt: draft.times.length > 0 ? Math.min(...draft.times) : undefined,
			endedAt:
				isLive || draft.times.length === 0
					? undefined
					: Math.max(...draft.times),
		};

		for (const { rowIndex, start, end } of draft.slices) {
			for (let pieceIndex = start; pieceIndex < end; pieceIndex++) {
				blockOfPiece[rowIndex][pieceIndex] = block;
			}
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
		const endRun = (end: number) => {
			if (runStart === undefined) {
				return;
			}
			const outputsBefore = split.pieces
				.slice(0, runStart)
				.filter((piece) => piece.kind === "output").length;
			items.push({
				type: "slice",
				slice: {
					rowIndex,
					pieceRange: { start: runStart, end },
					key: sliceKey(rowIndex, outputsBefore),
				},
			});
			runStart = undefined;
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
