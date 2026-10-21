import { Component } from "react";

type FocusItem = {
	/** The item's MessageScroller message ID. */
	itemKey: string;
	/** Persisted message IDs of the rows the item renders. */
	memberIds: readonly number[];
	/** A working block whose turn has ended. */
	isFold: boolean;
};

type FoldFocusRestorerProps = {
	/** Every timeline item, working blocks and row slices. */
	items: readonly FocusItem[];
};

type LostFocus = {
	control: Element;
	scrollerContent: Element;
	memberIds: readonly number[];
	newestMemberId: number;
};

/**
 * When a turn ends, its fold starts collapsed and takes in the turn's earlier
 * blocks and text, unmounting whatever the reader had focused there. This
 * focuses the summary of the fold that now holds the focused item's rows. A
 * class, so getSnapshotBeforeUpdate reads the focus before the commit.
 */
export class FoldFocusRestorer extends Component<FoldFocusRestorerProps> {
	getSnapshotBeforeUpdate(previous: FoldFocusRestorerProps): LostFocus | null {
		const control = document.activeElement;
		const itemElement = control?.closest("[data-message-id]");
		const source = previous.items.find(
			({ itemKey }) => itemKey === itemElement?.getAttribute("data-message-id"),
		);
		if (!control || !itemElement?.parentElement || !source) {
			return null;
		}
		return {
			control,
			scrollerContent: itemElement.parentElement,
			memberIds: source.memberIds,
			newestMemberId: Math.max(
				-1,
				...previous.items.flatMap(({ memberIds }) => memberIds),
			),
		};
	}

	componentDidUpdate(
		_previous: FoldFocusRestorerProps,
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
		// An item of only the live row has no persisted rows yet. The stream
		// persists as rows newer than any rendered before, so match on those.
		const holdsFocusedRows =
			focus.memberIds.length > 0
				? (id: number) => focus.memberIds.includes(id)
				: (id: number) => id > focus.newestMemberId;
		const fold = this.props.items.find(
			({ isFold, memberIds }) => isFold && memberIds.some(holdsFocusedRows),
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
