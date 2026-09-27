import {
	type MouseEventHandler,
	type PointerEventHandler,
	useRef,
} from "react";

type UseDragSelectOptions<T> = {
	items: readonly T[];
	selected: readonly T[];
	getId: (item: T) => string;
	isSelectable?: (item: T) => boolean;
	onChange: (selected: readonly T[]) => void;
};

type DragState = {
	anchor: number;
	select: boolean;
	initialIds: ReadonlySet<string>;
	/** Set once the pointer has entered a row other than the anchor. */
	moved: boolean;
};

/**
 * Selects a contiguous range of rows by holding the pointer down on one row's
 * checkbox and dragging across other rows. The range is set to match the
 * anchor row: dragging from an unchecked row selects, from a checked row
 * deselects. Dragging back over the range shrinks it again.
 *
 * Spread `getHandleProps` on each row's checkbox, `getRowProps` on each row
 * element, and `getContainerProps` on their common parent.
 */
export const useDragSelect = <T>({
	items,
	selected,
	getId,
	isSelectable,
	onChange,
}: UseDragSelectOptions<T>) => {
	const dragRef = useRef<DragState | null>(null);

	const applyRange = (to: number) => {
		const drag = dragRef.current;
		if (!drag) {
			return;
		}
		const ids = new Set(drag.initialIds);
		const [start, end] =
			drag.anchor < to ? [drag.anchor, to] : [to, drag.anchor];
		for (const item of items.slice(start, end + 1)) {
			if (isSelectable && !isSelectable(item)) {
				continue;
			}
			if (drag.select) {
				ids.add(getId(item));
			} else {
				ids.delete(getId(item));
			}
		}
		onChange(items.filter((item) => ids.has(getId(item))));
	};

	const endDrag = () => {
		window.removeEventListener("pointerup", endDrag);
		window.removeEventListener("pointercancel", endDrag);
		// The click that follows pointerup still needs to see `moved` so it can
		// be swallowed, so defer clearing until after it has been dispatched.
		const drag = dragRef.current;
		setTimeout(() => {
			if (dragRef.current === drag) {
				dragRef.current = null;
			}
		}, 0);
	};

	const getHandleProps = (
		index: number,
	): { onPointerDown: PointerEventHandler } => ({
		onPointerDown: (event) => {
			const item = items[index];
			if (
				event.button !== 0 ||
				event.pointerType === "touch" ||
				item === undefined ||
				(isSelectable && !isSelectable(item))
			) {
				return;
			}
			// Stops the browser from starting a text selection along the drag.
			event.preventDefault();
			const id = getId(item);
			dragRef.current = {
				anchor: index,
				select: !selected.some((s) => getId(s) === id),
				initialIds: new Set(selected.map(getId)),
				moved: false,
			};
			window.addEventListener("pointerup", endDrag);
			window.addEventListener("pointercancel", endDrag);
		},
	});

	const getRowProps = (
		index: number,
	): { onPointerEnter: PointerEventHandler } => ({
		onPointerEnter: () => {
			const drag = dragRef.current;
			if (!drag) {
				return;
			}
			if (index !== drag.anchor) {
				drag.moved = true;
			}
			if (drag.moved) {
				applyRange(index);
			}
		},
	});

	const getContainerProps = (): { onClickCapture: MouseEventHandler } => ({
		onClickCapture: (event) => {
			// Releasing back on the anchor row would otherwise register as a
			// click on it, navigating away or toggling the checkbox again.
			if (dragRef.current?.moved) {
				event.stopPropagation();
				dragRef.current = null;
			}
		},
	});

	return { getHandleProps, getRowProps, getContainerProps };
};
