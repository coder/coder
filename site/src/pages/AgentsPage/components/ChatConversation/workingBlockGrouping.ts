import { getVisibleContent } from "./messageHelpers";
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
	/** Distinct visible tools, not rows. */
	stepCount: number;
	failedCount: number;
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

const parseTimestamp = (value: string | undefined): number | undefined => {
	const time = Date.parse(value ?? "");
	return Number.isFinite(time) ? time : undefined;
};

type RowContent = ReturnType<typeof getVisibleContent>;

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

		content = getVisibleContent(options.liveBlocks, options.liveTools);
		if (content.visibleBlocks.length === 0) {
			return content;
		}
	} else {
		const { message, parsed } = row.entry;
		if (message.role !== "assistant" || parsed.hookNotices.length > 0) {
			return undefined;
		}

		content = getVisibleContent(parsed.blocks, parsed.tools);
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
		tools: Map<string, MergedTool>;
		anchorKey?: string;
		ordinal: number;
		containsLiveRow: boolean;
	};

	const drafts: Draft[] = [];
	let current: Draft | undefined;
	let anchorKey: string | undefined;
	let ordinal = 0;

	for (const [index, row] of rows.entries()) {
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

	const messageIdAfter = (lastRowIndex: number): number => {
		for (let i = lastRowIndex + 1; i < rows.length; i++) {
			const ids = rowMessageIds(rows[i]);
			if (ids.length > 0) {
				return Math.min(...ids);
			}
		}

		return Number.POSITIVE_INFINITY;
	};

	// Entries and blocks are both in ascending message ID order and block
	// spans never overlap, so one cursor walks the entries once.
	let entryIndex = 0;

	return blockDrafts.map((draft) => {
		const firstRowIndex = draft.rowIndices[0];
		const lastRowIndex = draft.rowIndices[draft.rowIndices.length - 1];
		const memberIds = draft.rowIndices.flatMap((i) => rowMessageIds(rows[i]));

		const isLive =
			options.isWorking &&
			(draft.containsLiveRow || lastRowIndex >= lastMessageRowIndex);

		// The span covers hidden tool-result messages up to the next row.
		const fromId = Math.min(...memberIds);
		const toId = messageIdAfter(lastRowIndex);
		while (
			entryIndex < entries.length &&
			entries[entryIndex].message.id < fromId
		) {
			entryIndex++;
		}

		const spanTimestamps: Array<string | undefined> = [];
		while (
			entryIndex < entries.length &&
			entries[entryIndex].message.id < toId
		) {
			spanTimestamps.push(...getPartTimestamps(entries[entryIndex]));
			entryIndex++;
		}

		if (draft.containsLiveRow) {
			spanTimestamps.push(options.streamState?.startedAt);
		}

		const times = spanTimestamps
			.map(parseTimestamp)
			.filter((time) => time !== undefined);

		const liveKey = `working:live:${draft.anchorKey ?? "head"}:${draft.ordinal}`;
		const key = isLive ? liveKey : `working:through:${rows[lastRowIndex].key}`;
		const tools = Array.from(draft.tools.values());

		return {
			key,
			liveKey,
			rowIndices: draft.rowIndices,
			memberIds,
			stepCount: tools.length,
			failedCount: tools.filter(
				(tool) => tool.isError || tool.status === "error",
			).length,
			isLive,
			isPartial: options.hasMoreMessages && firstRowIndex === 0,
			startedAt: times.length > 0 ? Math.min(...times) : undefined,
			endedAt: isLive || times.length === 0 ? undefined : Math.max(...times),
		};
	});
};
