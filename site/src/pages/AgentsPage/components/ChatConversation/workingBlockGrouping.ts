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
 * The answer is the content after a row's last reasoning or tool call, web
 * searches included; text before it is narration. Sources fold without
 * splitting the answer, since OpenAI streams citations between the deltas of
 * one text part.
 */
export const splitRowBlocks = (
	blocks: readonly RenderBlock[],
	tools: readonly MergedTool[],
) => {
	const visible = new Set(getVisibleContent(blocks, tools).visibleBlocks);
	const lastWorkIndex = blocks.findLastIndex(
		(block) =>
			visible.has(block) &&
			(block.type === "thinking" ||
				block.type === "tool" ||
				(block.type === "response" && block.beforeProviderTool)),
	);
	const work: RenderBlock[] = [];
	let answer: RenderBlock[] = [];
	let previous: RenderBlock | undefined;
	for (const [index, block] of blocks.entries()) {
		if (
			index <= lastWorkIndex ||
			!visible.has(block) ||
			block.type === "sources"
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

type RowContent = ReturnType<typeof getVisibleContent>;

type MemberRow = {
	content: RowContent;
	endsWithAnswer: boolean;
	searches: number;
};

/**
 * A row ending in an answer joins only when it also did work, and then closes
 * the block. A live row with no output yet is the turn working on its next
 * step.
 */
const getMemberRow = (
	row: TimelineRow,
	options: GroupWorkingBlocksOptions,
): MemberRow | undefined => {
	let content: RowContent;
	let searchResults: number;

	if (row.type === "live") {
		if (!options.isLiveRowCollapsible) {
			return undefined;
		}

		content = getVisibleContent(options.liveBlocks, options.liveTools);
		// Until the step persists, one flag stands in for its searches.
		searchResults = options.streamState?.providerToolRan ? 1 : 0;
	} else {
		const { message, parsed } = row.entry;
		if (message.role !== "assistant" || parsed.hookNotices.length > 0) {
			return undefined;
		}

		content = getVisibleContent(parsed.blocks, parsed.tools);
		// Results, not calls: a step can end with a provider call unanswered,
		// and the next step issues that call again.
		searchResults = (message.content ?? []).filter(isProviderToolResult).length;
	}

	const { visibleBlocks, visibleTools } = content;
	// A search counts even without citations, while some providers cite
	// sources without a search.
	const searches =
		searchResults ||
		visibleBlocks.filter((block) => block.type === "sources").length;
	if (visibleBlocks.length === 0) {
		return row.type === "live"
			? { content, endsWithAnswer: false, searches }
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
	if (answer.length === 0) {
		return { content, endsWithAnswer: false, searches };
	}

	return work.length > 0
		? { content, endsWithAnswer: true, searches }
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
		searches: number;
		endsWithAnswer: boolean;
		anchorKey?: string;
		ordinal: number;
		containsLiveRow: boolean;
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
			// The stream opens empty before every step. That row extends a
			// block that is already working but never starts one, so a turn's
			// first moments keep the plain thinking indicator.
			if (member.content.visibleBlocks.length === 0) {
				continue;
			}

			current = {
				rowIndices: [],
				toolIds: new Set(),
				searches: 0,
				endsWithAnswer: false,
				anchorKey,
				ordinal,
				containsLiveRow: false,
			};
			ordinal += 1;
			drafts.push(current);
		}

		current.rowIndices.push(index);
		current.containsLiveRow ||= row.type === "live";

		for (const tool of member.content.visibleTools) {
			current.toolIds.add(tool.id);
		}
		current.searches += member.searches;

		if (member.endsWithAnswer) {
			current.endsWithAnswer = true;
			current = undefined;
		}
	}

	const stepCountOf = (draft: Draft) => draft.toolIds.size + draft.searches;

	// Completed reasoning alone stays visible. The live turn folds from its first
	// reasoning, so thinking never shows and then vanishes once a tool call
	// arrives, and unfolds only when a turn without steps starts its answer.
	const blockDrafts = drafts.filter(
		(draft) =>
			stepCountOf(draft) > 0 ||
			(draft.containsLiveRow && options.isTurnActive && !draft.endsWithAnswer),
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

		// A persisted answer completes its block even while the chat still runs.
		const isLive =
			options.isWorking &&
			!(draft.endsWithAnswer && rows[lastRowIndex].type === "message") &&
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

		const spanTimestamps: string[] = [];
		while (
			entryIndex < entries.length &&
			entries[entryIndex].message.id < toId
		) {
			spanTimestamps.push(...getPartTimestamps(entries[entryIndex]));
			entryIndex++;
		}

		const streamStartedAt = options.streamState?.startedAt;
		if (draft.containsLiveRow && streamStartedAt !== undefined) {
			spanTimestamps.push(streamStartedAt);
		}

		const times = spanTimestamps.map((timestamp) => Date.parse(timestamp));

		const liveKey = `working:live:${draft.anchorKey ?? "head"}:${draft.ordinal}`;
		const key = isLive ? liveKey : `working:through:${rows[lastRowIndex].key}`;

		return {
			key,
			liveKey,
			rowIndices: draft.rowIndices,
			memberIds,
			stepCount: stepCountOf(draft),
			endsWithAnswer: draft.endsWithAnswer,
			isLive,
			isPartial: options.hasMoreMessages && firstRowIndex === 0,
			startedAt: times.length > 0 ? Math.min(...times) : undefined,
			endedAt: isLive || times.length === 0 ? undefined : Math.max(...times),
		};
	});
};
