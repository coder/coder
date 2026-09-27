import {
	type MouseEventHandler,
	type PointerEventHandler,
	useCallback,
	useEffect,
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
	anchorId: string;
	pointerId: number;
	select: boolean;
	initialIds: ReadonlySet<string>;
	moved: boolean;
};

/**
 * Mouse-drag selection starting at a row's checkbox. Dragging from a selected
 * row deselects the range; dragging back restores the original selection
 * outside it.
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
	const suppressClickRef = useRef(false);
	const clickTimeoutRef = useRef<number | undefined>(undefined);

	const cancelDrag = useCallback(() => {
		dragRef.current = null;
		suppressClickRef.current = false;
		window.clearTimeout(clickTimeoutRef.current);
	}, []);

	useEffect(() => {
		const endDrag = (event: PointerEvent) => {
			const drag = dragRef.current;
			if (!drag || event.pointerId !== drag.pointerId) {
				return;
			}
			cancelDrag();
			if (drag.moved) {
				// Ignore the click dispatched after pointerup, but not the next click
				// if releasing outside the table produces no click here.
				suppressClickRef.current = true;
				clickTimeoutRef.current = window.setTimeout(() => {
					suppressClickRef.current = false;
				}, 0);
			}
		};
		const onPointerCancel = (event: PointerEvent) => {
			if (event.pointerId === dragRef.current?.pointerId) {
				cancelDrag();
			}
		};

		window.addEventListener("pointerup", endDrag, true);
		window.addEventListener("pointercancel", onPointerCancel, true);
		window.addEventListener("blur", cancelDrag);
		return () => {
			window.removeEventListener("pointerup", endDrag, true);
			window.removeEventListener("pointercancel", onPointerCancel, true);
			window.removeEventListener("blur", cancelDrag);
			cancelDrag();
		};
	}, [cancelDrag]);

	const getHandleProps = (
		index: number,
	): { onPointerDown: PointerEventHandler<HTMLElement> } => ({
		onPointerDown: (event) => {
			const item = items[index];
			if (
				event.button !== 0 ||
				event.pointerType !== "mouse" ||
				event.ctrlKey ||
				item === undefined ||
				(isSelectable && !isSelectable(item))
			) {
				return;
			}
			// Preventing text selection also suppresses the browser's normal focus.
			event.preventDefault();
			event.currentTarget.focus({ preventScroll: true });
			cancelDrag();
			const id = getId(item);
			dragRef.current = {
				anchorId: id,
				pointerId: event.pointerId,
				select: !selected.some((s) => getId(s) === id),
				initialIds: new Set(selected.map(getId)),
				moved: false,
			};
		},
	});

	const getRowProps = (
		index: number,
	): { onPointerEnter: PointerEventHandler } => ({
		onPointerEnter: (event) => {
			const drag = dragRef.current;
			if (!drag || event.pointerId !== drag.pointerId) {
				return;
			}
			const anchor = items.findIndex((item) => getId(item) === drag.anchorId);
			if ((event.buttons & 1) === 0 || anchor === -1) {
				cancelDrag();
				return;
			}
			if (index !== anchor) {
				drag.moved = true;
			}
			if (!drag.moved) {
				return;
			}

			const ids = new Set(drag.initialIds);
			const [start, end] = anchor < index ? [anchor, index] : [index, anchor];
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
		},
	});

	const getContainerProps = (): { onClickCapture: MouseEventHandler } => ({
		onClickCapture: (event) => {
			if (suppressClickRef.current && event.detail !== 0) {
				event.preventDefault();
				event.stopPropagation();
				cancelDrag();
			}
		},
	});

	return { getHandleProps, getRowProps, getContainerProps };
};
