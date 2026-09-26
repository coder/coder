import { shouldRenderTool } from "../ChatElements/tools/toolVisibility";
import type { TimelineRow } from "./timelineRows";
import type {
	MergedTool,
	ParsedMessageEntry,
	RenderBlock,
	StreamState,
} from "./types";

/**
 * A run of consecutive assistant step rows that the timeline can fold into
 * one "Worked for" disclosure.
 */
export type WorkingBlock = {
	/** Newest row's key, stable across paging; live blocks use liveKey. */
	key: string;
	/**
	 * Identity the block had (or would have had) while live: its turn's
	 * opening row plus its position in the turn. Deterministic across the
	 * live-to-complete handoff, so expansion recorded before any step
	 * persisted can still be found afterwards.
	 */
	liveKey: string;
	rowIndices: number[];
	/** Distinct visible tools across the block. */
	stepCount: number;
	failedCount: number;
	/** The turn is still active and this block is where it is working. */
	isLive: boolean;
	/** Older history exists that may contain earlier rows of this block. */
	isPartial: boolean;
	/** Earliest and latest part timestamps (epoch ms); absent when unknown. */
	startedAt?: number;
	endedAt?: number;
};

export type GroupWorkingBlocksOptions = {
	hasMoreMessages: boolean;
	/** The turn is still producing output (any non-idle, non-failed phase). */
	isTurnActive: boolean;
	/**
	 * Whether the live row may be folded into the block: the turn is
	 * starting a step or streaming one. Retry, reconnect, and interrupt
	 * callouts render inside the live row, so it must stay visible outside
	 * any block in those phases.
	 */
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

const parseTimestamp = (value: string | undefined): number | undefined => {
	if (!value) {
		return undefined;
	}
	const time = Date.parse(value);
	return Number.isFinite(time) ? time : undefined;
};

type RowContent = {
	visibleBlocks: RenderBlock[];
	visibleTools: MergedTool[];
};

const getRowContent = (
	blocks: readonly RenderBlock[],
	tools: readonly MergedTool[],
): RowContent => {
	const visibleTools = tools.filter((tool) => shouldRenderTool(tool));
	const visibleIds = new Set(visibleTools.map((tool) => tool.id));
	const visibleBlocks = blocks.filter(
		(block) => block.type !== "tool" || visibleIds.has(block.id),
	);
	return { visibleBlocks, visibleTools };
};

/**
 * A step row is assistant output that ends in tool activity or reasoning
 * rather than an answer. Text that precedes a tool call is narration and
 * folds with it; text that ends a row is an answer and stays visible. A
 * live row with no output yet is the turn working on its next step.
 */
const getStepRowContent = (
	row: TimelineRow,
	options: GroupWorkingBlocksOptions,
): RowContent | undefined => {
	let content: RowContent;
	if (row.type === "live") {
		if (!options.isLiveRowCollapsible) {
			return undefined;
		}
		content = getRowContent(options.liveBlocks, options.liveTools);
		if (content.visibleBlocks.length === 0) {
			return content;
		}
	} else {
		const { message, parsed } = row.entry;
		if (message.role !== "assistant" || parsed.hookNotices.length > 0) {
			return undefined;
		}
		content = getRowContent(parsed.blocks, parsed.tools);
	}
	const { visibleBlocks, visibleTools } = content;
	if (visibleBlocks.length === 0) {
		return undefined;
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
	const last = visibleBlocks[visibleBlocks.length - 1];
	if (last.type !== "tool" && last.type !== "thinking") {
		return undefined;
	}
	return content;
};

/**
 * Part timestamps are the only reliable clock: message created_at is shared
 * across an insert batch, so it marks when a step was persisted, not when its
 * work started.
 */
const getPartTimestamps = (entry: ParsedMessageEntry) =>
	(entry.message.content ?? []).flatMap((part) => {
		if (part.type === "reasoning") {
			return [part.created_at, part.completed_at];
		}
		return part.type === "tool-call" || part.type === "tool-result"
			? [part.created_at]
			: [];
	});

const rowMessageIds = (row: TimelineRow): readonly number[] =>
	row.type === "live" ? [] : (row.entry.mergedFrom ?? [row.entry.message.id]);

/**
 * Groups consecutive step rows into working blocks. Timestamps come from the
 * raw entries, because the timeline rows already dropped tool-result messages
 * and merged read_file runs.
 */
export const groupWorkingBlocks = (
	rows: readonly TimelineRow[],
	entries: readonly ParsedMessageEntry[],
	options: GroupWorkingBlocksOptions,
): WorkingBlock[] => {
	type Draft = {
		rowIndices: number[];
		tools: Map<string, MergedTool>;
		anchorKey?: string;
		ordinal: number;
		containsLiveRow: boolean;
	};
	const drafts: Draft[] = [];
	let current: Draft | undefined;
	let anchorKey: string | undefined;
	let ordinal = 0;
	// A user message made only of context files or skills has no row but
	// still ends the turn before it.
	const userMessageIds = entries
		.filter((entry) => entry.message.role === "user")
		.map((entry) => entry.message.id);
	let lastMessageId = Number.NEGATIVE_INFINITY;
	for (const [index, row] of rows.entries()) {
		const ids = rowMessageIds(row);
		const hiddenUserId = userMessageIds.find(
			(id) => id > lastMessageId && ids.every((rowId) => id < rowId),
		);
		if (hiddenUserId !== undefined) {
			current = undefined;
			anchorKey = `message:${hiddenUserId}`;
			ordinal = 0;
		}
		if (ids.length > 0) {
			lastMessageId = Math.max(...ids);
		}
		const content = getStepRowContent(row, options);
		if (!content) {
			current = undefined;
			if (row.type === "message" && row.entry.message.role !== "assistant") {
				anchorKey = row.key;
				ordinal = 0;
			}
			continue;
		}
		if (!current) {
			// The stream opens empty before every step. That row extends a
			// block that is already working but never starts one, so a turn's
			// first moments keep the plain thinking indicator.
			if (content.visibleBlocks.length === 0) {
				continue;
			}
			current = {
				rowIndices: [],
				tools: new Map(),
				anchorKey,
				ordinal,
				containsLiveRow: false,
			};
			ordinal += 1;
			drafts.push(current);
		}
		current.rowIndices.push(index);
		current.containsLiveRow ||= row.type === "live";
		for (const tool of content.visibleTools) {
			current.tools.set(tool.id, tool);
		}
	}

	// A completed block is a run of tool activity; a reasoning-only row on
	// its own stays visible. The live turn folds from its first reasoning,
	// so thinking never shows and then vanishes once a tool call arrives.
	const blockDrafts = drafts.filter(
		(draft) =>
			draft.tools.size > 0 || (draft.containsLiveRow && options.isTurnActive),
	);

	const lastMessageRowIndex = rows.findLastIndex(
		(row) => row.type === "message",
	);
	const lastUserMessageId = Math.max(
		Number.NEGATIVE_INFINITY,
		...userMessageIds,
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

	return blockDrafts.map((draft) => {
		const firstRowIndex = draft.rowIndices[0];
		const lastRowIndex = draft.rowIndices[draft.rowIndices.length - 1];
		const memberIds = draft.rowIndices.flatMap((i) => rowMessageIds(rows[i]));
		// The newest block is still working unless a prompt, visible or hidden,
		// follows its last step.
		const lastMemberId = Math.max(...memberIds);
		const isLive =
			options.isTurnActive &&
			(draft.containsLiveRow ||
				(lastRowIndex >= lastMessageRowIndex &&
					lastMemberId > lastUserMessageId));

		// The span covers hidden tool-result messages up to the next row but
		// never passes the next prompt, whose provider-executed parts can
		// carry timestamps.
		const fromId = Math.min(...memberIds);
		const toId = Math.min(
			messageIdAfter(lastRowIndex),
			...userMessageIds.filter((id) => id > lastMemberId),
		);
		const times = [
			...entries
				.filter(({ message }) => message.id >= fromId && message.id < toId)
				.flatMap(getPartTimestamps),
			draft.containsLiveRow ? options.streamState?.startedAt : undefined,
		]
			.map(parseTimestamp)
			.filter((time) => time !== undefined);

		const liveKey = `working:live:${draft.anchorKey ?? "head"}:${draft.ordinal}`;
		const key = isLive ? liveKey : `working:through:${rows[lastRowIndex].key}`;
		const tools = Array.from(draft.tools.values());
		return {
			key,
			liveKey,
			rowIndices: draft.rowIndices,
			stepCount: tools.length,
			failedCount: tools.filter(
				(tool) => tool.isError || tool.status === "error",
			).length,
			isLive,
			// A loaded prompt, visible or hidden, bounds the block even when the
			// block's first row is the page's first row.
			isPartial:
				options.hasMoreMessages &&
				firstRowIndex === 0 &&
				draft.anchorKey === undefined,
			startedAt: times.length > 0 ? Math.min(...times) : undefined,
			endedAt: isLive || times.length === 0 ? undefined : Math.max(...times),
		};
	});
};
