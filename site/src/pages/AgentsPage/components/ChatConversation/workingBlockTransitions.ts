import { getVisibleContent } from "./messageHelpers";
import type { TimelineRow } from "./timelineRows";
import type { MergedTool, RenderBlock } from "./types";
import { getRowBlocks, type RowSection } from "./workingBlockGrouping";

/** Reply text that streamed as an answer and then folded into a block. */
type ReplyFold = {
	id: number;
	itemKey: string;
	blocks: readonly RenderBlock[];
};

type WorkingBlockTransitions = {
	/** Set once rows render, so blocks present on load never animate in. */
	hasRendered: boolean;
	/** The previous render showed live reply text outside any block. */
	replyShown: boolean;
	itemKeys: ReadonlySet<string>;
	/** Live blocks whose label slides in. */
	enteringLabels: ReadonlySet<string>;
	/** Queued oldest first; each block folds one at a time. */
	folds: readonly ReplyFold[];
	nextFoldId: number;
};

export const emptyWorkingBlockTransitions: WorkingBlockTransitions = {
	hasRendered: false,
	replyShown: false,
	itemKeys: new Set(),
	enteringLabels: new Set(),
	folds: [],
	nextFoldId: 0,
};

/** A rendered working block, as the timeline lays it out. */
type BlockView = {
	itemKey: string;
	isLive: boolean;
	expanded: boolean;
	rowIndices: readonly number[];
	/** The row whose answer renders after the block. */
	answerRowIndex: number | undefined;
};

type WorkingBlockFrame = {
	hasRows: boolean;
	blocks: readonly BlockView[];
	replyShown: boolean;
	/** The collapsed block that now holds the row whose reply was shown. */
	foldTarget:
		| {
				itemKey: string;
				blocks: readonly RenderBlock[];
				tools: readonly MergedTool[];
		  }
		| undefined;
};

const hasReplyText = (blocks: readonly RenderBlock[]): boolean =>
	blocks.some(
		(block) => block.type === "response" && block.text.trim().length > 0,
	);

type FrameInput = {
	rows: readonly TimelineRow[];
	blocks: readonly BlockView[];
	liveBlocks: readonly RenderBlock[];
	liveTools: readonly MergedTool[];
};

/** Whether live reply text renders outside a block, and which block took it in. */
export const getWorkingBlockFrame = ({
	rows,
	blocks,
	liveBlocks,
	liveTools,
}: FrameInput): WorkingBlockFrame => {
	const findBlock = (rowIndex: number) =>
		blocks.find((block) => block.rowIndices.includes(rowIndex));

	const liveRowIndex = rows.findIndex((row) => row.type === "live");
	let replyShown = false;
	if (liveRowIndex !== -1) {
		const block = findBlock(liveRowIndex);
		let section: RowSection | undefined;
		if (block !== undefined) {
			section = block.answerRowIndex === liveRowIndex ? "answer" : "work";
		}
		replyShown =
			section !== "work" &&
			hasReplyText(
				getRowBlocks(liveBlocks, liveTools, { section, hideAttachments: true }),
			);
	}

	// The step may persist in the same update its tool call arrives.
	const holderIndex = liveRowIndex !== -1 ? liveRowIndex : rows.length - 1;
	const holderRow = rows.at(holderIndex);
	const holder = findBlock(holderIndex);
	let foldTarget: WorkingBlockFrame["foldTarget"];
	if (
		holderRow &&
		holder &&
		!holder.expanded &&
		holder.answerRowIndex !== holderIndex
	) {
		foldTarget =
			holderRow.type === "live"
				? { itemKey: holder.itemKey, blocks: liveBlocks, tools: liveTools }
				: {
						itemKey: holder.itemKey,
						blocks: holderRow.entry.parsed.blocks,
						tools: holderRow.entry.parsed.tools,
					};
	}

	return { hasRows: rows.length > 0, blocks, replyShown, foldTarget };
};

// The answer a row showed before its trailing work. A provider tool call
// marks the text before it as work, but that text was shown as the answer.
const getFoldedReplyBlocks = (
	blocks: readonly RenderBlock[],
	tools: readonly MergedTool[],
): readonly RenderBlock[] => {
	const { visibleBlocks } = getVisibleContent(blocks, tools);
	const end =
		visibleBlocks.findLastIndex(
			(block) => block.type !== "tool" && block.type !== "thinking",
		) + 1;
	const shown = visibleBlocks
		.slice(0, end)
		.map((block, index) =>
			index === end - 1 && block.type === "response"
				? { ...block, beforeProviderTool: false }
				: block,
		);
	return getRowBlocks(shown, tools, {
		section: "answer",
		hideAttachments: true,
	});
};

const setsEqual = (a: ReadonlySet<string>, b: ReadonlySet<string>) =>
	a.size === b.size && [...a].every((value) => b.has(value));

/** Returns `state` itself when nothing changed, so callers can set it during render. */
export const reconcileWorkingBlockTransitions = (
	state: WorkingBlockTransitions,
	frame: WorkingBlockFrame,
): WorkingBlockTransitions => {
	if (!frame.hasRows) {
		return state;
	}

	const itemKeys = new Set(frame.blocks.map((block) => block.itemKey));
	const collapsedKeys = new Set(
		frame.blocks
			.filter((block) => !block.expanded)
			.map((block) => block.itemKey),
	);

	let enteringLabels = state.enteringLabels;
	let folds = state.folds;
	let nextFoldId = state.nextFoldId;

	if (state.hasRendered) {
		const added = frame.blocks.filter(
			(block) => block.isLive && !state.itemKeys.has(block.itemKey),
		);
		if (added.length > 0) {
			enteringLabels = new Set([
				...enteringLabels,
				...added.map((block) => block.itemKey),
			]);
		}

		const target = frame.foldTarget;
		if (state.replyShown && !frame.replyShown && target) {
			const blocks = getFoldedReplyBlocks(target.blocks, target.tools);
			if (hasReplyText(blocks)) {
				folds = [...folds, { id: nextFoldId, itemKey: target.itemKey, blocks }];
				nextFoldId += 1;
			}
		}
	}

	// An expanded block already shows the text, and a gone block has no label.
	if (folds.some((fold) => !collapsedKeys.has(fold.itemKey))) {
		folds = folds.filter((fold) => collapsedKeys.has(fold.itemKey));
	}
	if ([...enteringLabels].some((key) => !itemKeys.has(key))) {
		enteringLabels = new Set(
			[...enteringLabels].filter((key) => itemKeys.has(key)),
		);
	}

	const sameItemKeys = setsEqual(state.itemKeys, itemKeys);
	if (
		state.hasRendered &&
		state.replyShown === frame.replyShown &&
		sameItemKeys &&
		enteringLabels === state.enteringLabels &&
		folds === state.folds
	) {
		return state;
	}

	return {
		hasRendered: true,
		replyShown: frame.replyShown,
		itemKeys: sameItemKeys ? state.itemKeys : itemKeys,
		enteringLabels,
		folds,
		nextFoldId,
	};
};

export const finishLabelEntry = (
	state: WorkingBlockTransitions,
	itemKey: string,
): WorkingBlockTransitions => {
	if (!state.enteringLabels.has(itemKey)) {
		return state;
	}
	const enteringLabels = new Set(state.enteringLabels);
	enteringLabels.delete(itemKey);
	return { ...state, enteringLabels };
};

export const finishFold = (
	state: WorkingBlockTransitions,
	id: number,
): WorkingBlockTransitions =>
	state.folds.some((fold) => fold.id === id)
		? { ...state, folds: state.folds.filter((fold) => fold.id !== id) }
		: state;
