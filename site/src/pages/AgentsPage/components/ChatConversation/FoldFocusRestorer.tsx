import { Component } from "react";
import type { TimelineRow } from "./timelineRows";
import {
	rowMessageIds,
	type WorkingBlock,
	type WorkingBlockLayout,
} from "./workingBlockGrouping";

type FocusItem = {
	/** Matches the item element's data-message-id. */
	itemKey: string;
	/** Persisted message IDs of the rows the item renders. */
	memberIds: readonly number[];
	/** A working block whose turn has ended. */
	isFold: boolean;
};

/** Each item's scroller key and rows, marking ended-turn blocks as folds. */
export const foldFocusItems = (
	items: WorkingBlockLayout["items"],
	rows: readonly TimelineRow[],
	getItemKey: (block: WorkingBlock) => string,
): FocusItem[] =>
	items.map((item) =>
		item.type === "block"
			? {
					itemKey: getItemKey(item.block),
					memberIds: item.block.memberIds,
					isFold: item.block.turnKey !== undefined,
				}
			: {
					itemKey: item.slice.key,
					memberIds: rowMessageIds(rows[item.slice.rowIndex]),
					isFold: false,
				},
	);

type FoldFocusRestorerProps = {
	/** Every timeline item, working blocks and row slices. */
	items: readonly FocusItem[];
};

type LostFocus = {
	control: Element;
	scrollerContent: Element;
	memberIds: readonly number[];
};

/**
 * An ended turn's fold unmounts the content the reader had focused. A class,
 * so getSnapshotBeforeUpdate reads that focus before the commit.
 */
export class FoldFocusRestorer extends Component<FoldFocusRestorerProps> {
	getSnapshotBeforeUpdate(previous: FoldFocusRestorerProps): LostFocus | null {
		const control = document.activeElement;
		const itemElement = control?.closest("[data-message-id]");
		const focusedItem = previous.items.find(
			({ itemKey }) => itemKey === itemElement?.getAttribute("data-message-id"),
		);
		if (!control || !itemElement?.parentElement || !focusedItem) {
			return null;
		}
		return {
			control,
			scrollerContent: itemElement.parentElement,
			memberIds: focusedItem.memberIds,
		};
	}

	componentDidUpdate(
		previous: FoldFocusRestorerProps,
		_state: unknown,
		focus: LostFocus | null,
	) {
		if (
			!focus ||
			focus.control.isConnected ||
			document.activeElement !== document.body
		) {
			return;
		}
		let belongsToFocusedItem = (id: number) => focus.memberIds.includes(id);
		if (focus.memberIds.length === 0) {
			// An item of only the live row has no persisted rows yet. The
			// stream persists as rows newer than any rendered before.
			const newestId = previous.items.reduce(
				(newest, { memberIds }) => Math.max(newest, ...memberIds),
				-1,
			);
			belongsToFocusedItem = (id) => id > newestId;
		}
		const fold = this.props.items.find(
			({ isFold, memberIds }) => isFold && memberIds.some(belongsToFocusedItem),
		);
		if (!fold) {
			return;
		}
		const foldElement = Array.from(focus.scrollerContent.children).find(
			(element) => element.getAttribute("data-message-id") === fold.itemKey,
		);
		// A block's summary is its first disclosure button.
		foldElement
			?.querySelector<HTMLElement>("button[aria-expanded]")
			?.focus({ preventScroll: true });
	}

	render() {
		return null;
	}
}
