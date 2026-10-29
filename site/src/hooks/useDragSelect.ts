import type React from "react";
import { useCallback, useEffect, useRef } from "react";

type UseDragSelectOptions<T> = {
	items: readonly T[];
	selected: readonly T[];
	getId: (item: T) => string;
	isSelectable: (item: T) => boolean;
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
 * Dragging from a selected checkbox deselects; reversing restores selection outside the range.
 * Spread `getHandleProps` on checkboxes, `getRowProps` on rows, and
 * `getContainerProps` on their common parent.
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
		id: string,
	): { onPointerDown: React.PointerEventHandler<HTMLElement> } => ({
		onPointerDown: (event) => {
			const item = items.find((item) => getId(item) === id);
			if (
				event.button !== 0 ||
				event.pointerType !== "mouse" ||
				event.ctrlKey ||
				item === undefined ||
				!isSelectable(item)
			) {
				return;
			}
			// Preventing text selection also suppresses the browser's normal focus.
			event.preventDefault();
			event.currentTarget.focus({ preventScroll: true });
			cancelDrag();
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
		id: string,
	): { onPointerEnter: React.PointerEventHandler } => ({
		onPointerEnter: (event) => {
			const drag = dragRef.current;
			if (!drag || event.pointerId !== drag.pointerId) {
				return;
			}
			if (
				(event.buttons & 1) === 0 ||
				!items.some((item) => getId(item) === drag.anchorId)
			) {
				cancelDrag();
				return;
			}
			if (id !== drag.anchorId) {
				drag.moved = true;
			}
			if (!drag.moved) {
				return;
			}

			const ids = new Set(drag.initialIds);
			const endpoints = new Set([drag.anchorId, id]);
			let inRange = false;
			for (const item of items) {
				const itemId = getId(item);
				if (endpoints.delete(itemId)) {
					inRange = true;
				}
				if (inRange && isSelectable(item)) {
					if (drag.select) {
						ids.add(itemId);
					} else {
						ids.delete(itemId);
					}
				}
				if (endpoints.size === 0) {
					break;
				}
			}
			onChange(items.filter((item) => ids.has(getId(item))));
		},
	});

	const getContainerProps = (): {
		onClickCapture: React.MouseEventHandler;
	} => ({
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
