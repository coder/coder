import { shouldRenderTool } from "../ChatElements/tools/toolVisibility";
import {
	asString,
	formatModelIntentLabel,
	humanizeMCPToolName,
	parseArgs,
} from "../ChatElements/tools/utils";
import { getThinkingHeading } from "./thinkingTitle";
import type { TimelineRow } from "./timelineRows";
import type {
	MergedTool,
	ParsedMessageContent,
	ParsedMessageEntry,
	RenderBlock,
	StreamState,
} from "./types";

/**
 * A run of consecutive assistant step rows that the timeline can fold into
 * one "Worked for" disclosure. Rows are indices into the timeline rows the
 * block was computed from.
 */
export type WorkingBlock = {
	/**
	 * React key. Complete blocks key off their newest row, which pagination
	 * never changes; the live block uses liveKey so appended steps never
	 * remount it.
	 */
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
	/** "stopped" when a step was interrupted or the chat ended in an error. */
	outcome?: "completed" | "stopped";
	/** Older history exists that may contain earlier rows of this block. */
	isPartial: boolean;
	/** Earliest and latest part timestamps (epoch ms); absent when unknown. */
	startedAt?: number;
	endedAt?: number;
	/**
	 * The live block's newest describable step: tool intent, command, name,
	 * or reasoning heading. Plain thinking keeps the previous value.
	 */
	activity?: string;
};

export type GroupWorkingBlocksOptions = {
	hasMoreMessages: boolean;
	/** The turn is still producing output (any non-idle, non-failed phase). */
	isTurnActive: boolean;
	/** The chat's last turn ended in an error. */
	isTurnStopped?: boolean;
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

/**
 * Tools that need the user's attention or record a transcript boundary.
 * Rows containing them are never folded away.
 */
const UNCOLLAPSIBLE_TOOLS: ReadonlySet<string> = new Set([
	"ask_user_question",
	"propose_plan",
	"chat_summarized",
	"chat_cleared",
]);

// Written by chatd onto a tool call cut off by an interruption; see
// interruptedToolResultErrorMessage in coderd/x/chatd/message_conversion.go.
const INTERRUPTED_TOOL_RESULT_ERROR =
	"tool call was interrupted before it produced a result";

const isInterruptedTool = (tool: MergedTool): boolean => {
	const result = tool.result as { error?: unknown } | null | undefined;
	return (
		typeof result === "object" &&
		result !== null &&
		result.error === INTERRUPTED_TOOL_RESULT_ERROR
	);
};

const parseTimestamp = (value: string | undefined): number | undefined => {
	if (!value) {
		return undefined;
	}
	const time = Date.parse(value);
	return Number.isFinite(time) ? time : undefined;
};

/**
 * Splits a first step's leading narration from the tool work it introduces
 * so the lead-in can render above the fold. No opening when the row starts
 * with a tool or is text only.
 */
export const splitOpeningBlocks = (
	blocks: readonly RenderBlock[],
): { opening?: RenderBlock[]; rest: readonly RenderBlock[] } => {
	let leading = 0;
	while (leading < blocks.length && blocks[leading].type === "response") {
		leading += 1;
	}
	if (leading === 0 || leading === blocks.length) {
		return { rest: blocks };
	}
	return { opening: blocks.slice(0, leading), rest: blocks.slice(leading) };
};

export const splitOpeningNarration = (
	parsed: ParsedMessageContent,
): { opening?: ParsedMessageContent; rest: ParsedMessageContent } => {
	const { opening, rest } = splitOpeningBlocks(parsed.blocks);
	if (!opening) {
		return { rest: parsed };
	}
	return {
		opening: {
			...parsed,
			blocks: opening,
			tools: [],
			toolCalls: [],
			toolResults: [],
			sources: [],
			hookNotices: [],
		},
		rest: { ...parsed, blocks: [...rest] },
	};
};

export const formatWorkingDuration = (milliseconds: number): string => {
	const totalSeconds = Number.isFinite(milliseconds)
		? Math.max(0, Math.floor(milliseconds / 1000))
		: 0;
	const hours = Math.floor(totalSeconds / 3600);
	const minutes = Math.floor((totalSeconds % 3600) / 60);
	const seconds = totalSeconds % 60;
	if (hours > 0) {
		return `${hours}h ${minutes}m`;
	}
	if (minutes > 0) {
		return `${minutes}m ${seconds}s`;
	}
	return `${seconds}s`;
};

type RowContent = {
	visibleBlocks: RenderBlock[];
	visibleTools: MergedTool[];
};

const getToolActivity = (tool: MergedTool): string => {
	const intent = formatModelIntentLabel(tool.modelIntent);
	if (intent) {
		return intent;
	}
	if (tool.name === "execute") {
		const command = asString(parseArgs(tool.args)?.command).trim();
		if (command) {
			return command;
		}
	}
	return humanizeMCPToolName("", tool.name);
};

const getRowActivity = (content: RowContent): string | undefined => {
	const last = content.visibleBlocks[content.visibleBlocks.length - 1];
	if (!last) {
		return undefined;
	}
	if (last.type === "tool") {
		const tool = content.visibleTools.find((tool) => tool.id === last.id);
		return tool ? getToolActivity(tool) : undefined;
	}
	if (last.type === "thinking") {
		return getThinkingHeading(last.text);
	}
	return undefined;
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
 *
 * Streaming text cannot be told from narration until the step ends, so a
 * live row that continues a block folds even while it ends in text; a final
 * answer leaves the block once it persists.
 */
const isStepRow = (
	row: TimelineRow,
	options: GroupWorkingBlocksOptions,
	continuesBlock: boolean,
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
	if (last.type === "response" && row.type === "live" && continuesBlock) {
		return content;
	}
	if (last.type !== "tool" && last.type !== "thinking") {
		return undefined;
	}
	return content;
};

type MessageSpan = { id: number; startedAt?: number; endedAt?: number };

/**
 * Part timestamps are the only reliable clock: message created_at is shared
 * across an insert batch, so it marks when a step was persisted, not when its
 * work started.
 */
const getMessageSpan = (entry: ParsedMessageEntry): MessageSpan => {
	let startedAt: number | undefined;
	let endedAt: number | undefined;
	const observe = (value: string | undefined) => {
		const time = parseTimestamp(value);
		if (time === undefined) {
			return;
		}
		startedAt = startedAt === undefined ? time : Math.min(startedAt, time);
		endedAt = endedAt === undefined ? time : Math.max(endedAt, time);
	};
	for (const part of entry.message.content ?? []) {
		switch (part.type) {
			case "tool-call":
			case "tool-result":
				observe(part.created_at);
				break;
			case "reasoning":
				observe(part.created_at);
				observe(part.completed_at);
				break;
			default:
				break;
		}
	}
	return { id: entry.message.id, startedAt, endedAt };
};

const rowMessageIds = (row: TimelineRow): readonly number[] =>
	row.type === "live" ? [] : (row.entry.mergedFrom ?? [row.entry.message.id]);

const rowKey = (row: TimelineRow): string => row.key;

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
	const spans = entries.map(getMessageSpan).sort((a, b) => a.id - b.id);
	const firstSpanIndexAtOrAfter = (id: number): number => {
		let low = 0;
		let high = spans.length;
		while (low < high) {
			const mid = (low + high) >>> 1;
			if (spans[mid].id < id) {
				low = mid + 1;
			} else {
				high = mid;
			}
		}
		return low;
	};

	type Draft = {
		rowIndices: number[];
		tools: Map<string, MergedTool>;
		anchorKey?: string;
		ordinal: number;
		containsLiveRow: boolean;
		activity?: string;
	};
	const drafts: Draft[] = [];
	let current: Draft | undefined;
	let anchorKey: string | undefined;
	let ordinal = 0;
	for (const [index, row] of rows.entries()) {
		const content = isStepRow(row, options, current !== undefined);
		if (!content) {
			current = undefined;
			if (row.type === "message" && row.entry.message.role !== "assistant") {
				anchorKey = rowKey(row);
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
		current.activity = getRowActivity(content) ?? current.activity;
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

	return blockDrafts.map((draft) => {
		const firstRowIndex = draft.rowIndices[0];
		const lastRowIndex = draft.rowIndices[draft.rowIndices.length - 1];
		const memberIds = draft.rowIndices.flatMap((i) => rowMessageIds(rows[i]));
		const isLive =
			options.isTurnActive &&
			(draft.containsLiveRow || lastRowIndex >= lastMessageRowIndex);
		const isNewest = lastRowIndex >= lastMessageRowIndex;
		const tools = Array.from(draft.tools.values());
		const wasStopped =
			(isNewest && options.isTurnStopped) || tools.some(isInterruptedTool);
		const outcome: WorkingBlock["outcome"] = isLive
			? undefined
			: wasStopped
				? "stopped"
				: "completed";

		let startedAt: number | undefined;
		let endedAt: number | undefined;
		const observe = (time: number | undefined) => {
			if (time === undefined) {
				return;
			}
			startedAt = startedAt === undefined ? time : Math.min(startedAt, time);
			endedAt = endedAt === undefined ? time : Math.max(endedAt, time);
		};
		if (memberIds.length > 0) {
			// Hidden tool-result messages sit between the block's rows and the
			// next visible row, so the span runs to the next row's message.
			const fromId = Math.min(...memberIds);
			const toId = messageIdAfter(lastRowIndex);
			for (
				let i = firstSpanIndexAtOrAfter(fromId);
				i < spans.length && spans[i].id < toId;
				i++
			) {
				observe(spans[i].startedAt);
				observe(spans[i].endedAt);
			}
		}
		if (draft.containsLiveRow) {
			observe(parseTimestamp(options.streamState?.startedAt));
		}

		const liveKey = `working:live:${draft.anchorKey ?? "head"}:${draft.ordinal}`;
		const key = isLive
			? liveKey
			: `working:through:${rowKey(rows[lastRowIndex])}`;
		return {
			key,
			liveKey,
			rowIndices: draft.rowIndices,
			stepCount: tools.length,
			failedCount: tools.filter(
				(tool) => tool.isError || tool.status === "error",
			).length,
			isLive,
			outcome,
			isPartial: options.hasMoreMessages && firstRowIndex === 0,
			startedAt,
			endedAt: isLive ? undefined : endedAt,
			activity: isLive ? draft.activity : undefined,
		};
	});
};
