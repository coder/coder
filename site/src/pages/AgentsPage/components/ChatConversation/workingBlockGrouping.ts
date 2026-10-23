import type * as TypesGen from "#/api/typesGenerated";
import { appendTextBlock } from "./blockUtils";
import { getVisibleContent, isProviderToolResult } from "./messageHelpers";
import type { TimelineRow } from "./timelineRows";
import type {
	MergedTool,
	ParsedMessageEntry,
	RenderBlock,
	StreamState,
} from "./types";

export type WorkingBlock = {
	/** Follows the newest row, so prepending older history never changes it. */
	key: string;
	/**
	 * Unchanged across the live-to-complete handoff, so expansion recorded
	 * while live survives.
	 */
	liveKey: string;
	rowIndices: number[];
	/**
	 * Persisted member message IDs, oldest first. A merged read_file row
	 * contributes every message it merged, so unlike row keys the list grows
	 * at the front when history is prepended into that row.
	 */
	memberIds: number[];
	/** Distinct visible tools plus web searches, not rows. */
	stepCount: number;
	/** The last row's answer renders after the block; only its work folds in. */
	endsWithAnswer: boolean;
	isLive: boolean;
	/** Unloaded older history may hold earlier rows of this block. */
	isPartial: boolean;
	/** Epoch ms. */
	startedAt?: number;
	endedAt?: number;
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
 * A row that ends a working block renders its work inside the block and its
 * answer after it.
 */
export type RowSection = "work" | "answer";

/**
 * The answer is the content after a row's last reasoning, tool call, or
 * labeled narration, web searches included; text before it is narration.
 * Sources fold without splitting the answer, since OpenAI streams citations
 * between the deltas of one text part.
 */
const splitRowBlocks = (
	blocks: readonly RenderBlock[],
	tools: readonly MergedTool[],
) => {
	const visible = new Set(getVisibleContent(blocks, tools).visibleBlocks);
	const lastWorkIndex = blocks.findLastIndex(
		(block) =>
			visible.has(block) &&
			(block.type === "thinking" ||
				block.type === "tool" ||
				(block.type === "response" &&
					(block.beforeProviderTool || block.narration))),
	);
	const work: RenderBlock[] = [];
	let answer: RenderBlock[] = [];
	let previous: RenderBlock | undefined;
	for (const [index, block] of blocks.entries()) {
		if (
			index <= lastWorkIndex ||
			!visible.has(block) ||
			block.type === "sources" ||
			// A file belongs to the step that attached it.
			block.type === "file"
		) {
			work.push(block);
		} else if (block.type === "response" && previous?.type === "response") {
			answer = appendTextBlock(answer, "response", block.text);
		} else {
			answer = [...answer, block];
		}
		if (block.type !== "sources") {
			previous = block;
		}
	}

	return { work, answer } satisfies Record<RowSection, RenderBlock[]>;
};

type GetRowBlocksOptions = {
	section: RowSection | undefined;
	hideAttachments: boolean;
};

/**
 * A row's blocks, or its work or answer section when the row ends a working
 * block. Files that hideAttachments drops from agent rows are listed under
 * Attachments once their step persists.
 */
export const getRowBlocks = (
	blocks: readonly RenderBlock[],
	tools: readonly MergedTool[],
	{ section, hideAttachments }: GetRowBlocksOptions,
): readonly RenderBlock[] => {
	const sectionBlocks =
		section === undefined ? blocks : splitRowBlocks(blocks, tools)[section];
	return hideAttachments
		? sectionBlocks.filter((block) => block.type !== "file")
		: sectionBlocks;
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

type RowContent = ReturnType<typeof getVisibleContent>;

type MemberRow = {
	content: RowContent;
	endsWithAnswer: boolean;
	showsWork: boolean;
	narrates: boolean;
};

/**
 * A row ending in an answer joins only when it also did work, even a search
 * without citations, and then closes the block. A live row with no output yet
 * is the turn working on its next step.
 */
const getMemberRow = (
	row: TimelineRow,
	options: GroupWorkingBlocksOptions,
): MemberRow | undefined => {
	let content: RowContent;
	let searched: boolean;

	if (row.type === "live") {
		if (!options.isLiveRowCollapsible) {
			return undefined;
		}

		content = getVisibleContent(options.liveBlocks, options.liveTools);
		searched = options.streamState?.providerToolRan ?? false;
	} else {
		const { message, parsed } = row.entry;
		if (message.role !== "assistant" || parsed.hookNotices.length > 0) {
			return undefined;
		}

		content = getVisibleContent(parsed.blocks, parsed.tools);
		searched = (message.content ?? []).some(isProviderToolResult);
	}

	const { visibleBlocks, visibleTools } = content;
	if (visibleBlocks.length === 0) {
		return row.type === "live"
			? { content, endsWithAnswer: false, showsWork: false, narrates: false }
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

	const { work, answer } = splitRowBlocks(visibleBlocks, visibleTools);
	const narrates = work.some(
		(block) => block.type === "response" && block.narration,
	);
	if (answer.length === 0) {
		return { content, endsWithAnswer: false, showsWork: true, narrates };
	}

	return work.length > 0 || searched
		? { content, endsWithAnswer: true, showsWork: work.length > 0, narrates }
		: undefined;
};

/**
 * Part timestamps are the only reliable clock: message created_at is shared
 * across an insert batch, so it marks when a step was persisted, not when its
 * work started.
 */
const getPartTimestamps = (entry: ParsedMessageEntry): string[] =>
	(entry.message.content ?? [])
		.flatMap((part) => {
			if (part.type === "reasoning") {
				return [part.created_at, part.completed_at];
			}

			return part.type === "tool-call" || part.type === "tool-result"
				? [part.created_at]
				: [];
		})
		.filter((timestamp) => timestamp !== undefined);

/**
 * Results, not calls: a step can end with a provider call unanswered, and the
 * next step issues that call again.
 */
const countProviderResults = (message: TypesGen.ChatMessage): number =>
	(message.content ?? []).filter(isProviderToolResult).length;

const rowMessageIds = (row: TimelineRow): readonly number[] =>
	row.type === "live" ? [] : (row.entry.mergedFrom ?? [row.entry.message.id]);

/**
 * Timestamps come from the raw entries, because the timeline rows already
 * dropped tool-result messages and merged read_file runs.
 */
export const groupWorkingBlocks = (
	rows: readonly TimelineRow[],
	entries: readonly ParsedMessageEntry[],
	options: GroupWorkingBlocksOptions,
): WorkingBlock[] => {
	type Draft = {
		rowIndices: number[];
		toolIds: Set<string>;
		sourceBlocks: number;
		endsWithAnswer: boolean;
		anchorKey?: string;
		ordinal: number;
		containsLiveRow: boolean;
		narrates: boolean;
	};

	const drafts: Draft[] = [];
	let current: Draft | undefined;
	let anchorKey: string | undefined;
	let ordinal = 0;

	for (const [index, row] of rows.entries()) {
		const member = getMemberRow(row, options);
		if (!member) {
			current = undefined;
			if (row.type === "message" && row.entry.message.role !== "assistant") {
				anchorKey = row.key;
				ordinal = 0;
			}
			continue;
		}

		if (!current) {
			// Rows that render nothing, like the empty stream before each step or
			// an uncited search, extend a block but never start one: a turn's
			// first moments keep the thinking indicator, and no block is empty.
			if (!member.showsWork) {
				continue;
			}

			current = {
				rowIndices: [],
				toolIds: new Set(),
				sourceBlocks: 0,
				endsWithAnswer: false,
				anchorKey,
				ordinal,
				containsLiveRow: false,
				narrates: false,
			};
			ordinal += 1;
			drafts.push(current);
		}

		current.rowIndices.push(index);
		current.containsLiveRow ||= row.type === "live";
		current.narrates ||= member.narrates;

		for (const tool of member.content.visibleTools) {
			current.toolIds.add(tool.id);
		}
		current.sourceBlocks += member.content.visibleBlocks.filter(
			(block) => block.type === "sources",
		).length;

		if (member.endsWithAnswer) {
			current.endsWithAnswer = true;
			current = undefined;
		}
	}

	const lastMessageRowIndex = rows.findLastIndex(
		(row) => row.type === "message",
	);

	const messageIdAfter = (lastRowIndex: number): number => {
		for (let i = lastRowIndex + 1; i < rows.length; i++) {
			const ids = rowMessageIds(rows[i]);
			if (ids.length > 0) {
				return Math.min(...ids);
			}
		}

		return Number.POSITIVE_INFINITY;
	};

	// Each span starts where the previous one ended, so no entry counts toward
	// two blocks.
	let spanStartId = Number.NEGATIVE_INFINITY;
	const blocks: WorkingBlock[] = [];

	for (const draft of drafts) {
		const firstRowIndex = draft.rowIndices[0];
		const lastRowIndex = draft.rowIndices[draft.rowIndices.length - 1];
		const memberIds = draft.rowIndices.flatMap((i) => rowMessageIds(rows[i]));

		// A persisted answer completes its block even while the chat still runs.
		const isLive =
			options.isWorking &&
			!(draft.endsWithAnswer && rows[lastRowIndex].type === "message") &&
			(draft.containsLiveRow || lastRowIndex >= lastMessageRowIndex);

		// The span covers hidden tool-result messages up to the next row and,
		// before the first row, searches that rendered nothing since the
		// previous row.
		const fromId = Math.min(...memberIds);
		const toId = messageIdAfter(lastRowIndex);
		const previousRowId = Math.max(
			...(firstRowIndex > 0 ? rowMessageIds(rows[firstRowIndex - 1]) : []),
		);

		const spanEntries = entries.filter(({ message }) => {
			const { id } = message;
			if (id < spanStartId || id >= toId) {
				return false;
			}
			return (
				id >= fromId ||
				(id > previousRowId && countProviderResults(message) > 0)
			);
		});
		spanStartId = toId;

		// Until the step persists, one flag stands in for its searches.
		const searches = spanEntries.reduce(
			(count, { message }) => count + countProviderResults(message),
			draft.containsLiveRow && options.streamState?.providerToolRan ? 1 : 0,
		);

		// Source blocks count as searches only when no provider result does:
		// some providers cite without a search, and an interrupt commits a
		// search's result apart from the answer citing it.
		const stepCount = draft.toolIds.size + (searches || draft.sourceBlocks);

		// Completed reasoning alone stays visible. The live turn folds from its
		// first reasoning, so thinking never shows and then vanishes once a tool
		// call arrives, and unfolds only when a turn without steps answers.
		// Labeled narration keeps its block so it never reads as the answer.
		const foldsLiveTurn =
			draft.containsLiveRow && options.isTurnActive && !draft.endsWithAnswer;
		const keepsBlock = stepCount > 0 || foldsLiveTurn || draft.narrates;
		if (!keepsBlock) {
			continue;
		}

		const streamStartedAt = options.streamState?.startedAt;
		const times = [
			...spanEntries.flatMap(getPartTimestamps),
			...(draft.containsLiveRow && streamStartedAt !== undefined
				? [streamStartedAt]
				: []),
		].map((timestamp) => Date.parse(timestamp));

		const liveKey = `working:live:${draft.anchorKey ?? "head"}:${draft.ordinal}`;
		const key = isLive ? liveKey : `working:through:${rows[lastRowIndex].key}`;

		blocks.push({
			key,
			liveKey,
			rowIndices: draft.rowIndices,
			memberIds,
			stepCount,
			endsWithAnswer: draft.endsWithAnswer,
			isLive,
			isPartial: options.hasMoreMessages && firstRowIndex === 0,
			startedAt: times.length > 0 ? Math.min(...times) : undefined,
			endedAt: isLive || times.length === 0 ? undefined : Math.max(...times),
		});
	}

	return blocks;
};
