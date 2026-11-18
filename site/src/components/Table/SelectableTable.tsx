import type React from "react";
import {
	createContext,
	useCallback,
	useContext,
	useEffect,
	useRef,
} from "react";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { Table, TableRow } from "./Table";

type SelectableRow = {
	id: string;
	disabled?: boolean;
};

type SelectableTableProps = React.ComponentProps<typeof Table> & {
	/** Rows in display order, with unique IDs within this table. */
	rows: readonly SelectableRow[];
	selectedIds: ReadonlySet<string>;
	onSelectionChange: (selectedIds: Set<string>) => void;
};

type SelectionAnchor = {
	id: string;
	checked: boolean;
	range?: {
		initialIds: ReadonlySet<string>;
		expectedIds: ReadonlySet<string>;
	};
};

type DragState = {
	anchorId: string;
	pointerId: number;
	checked: boolean;
	initialIds: ReadonlySet<string>;
	expectedIds: ReadonlySet<string>;
	// Consecutive pointer events can arrive before a selection update renders.
	renderedIds: ReadonlySet<string>;
	moved: boolean;
	canceled: boolean;
};

type SelectionContextValue = {
	rows: ReadonlyMap<string, SelectableRow>;
	selectedIds: ReadonlySet<string>;
	onSelectionChange: (selectedIds: Set<string>) => void;
	setRowChecked: (id: string, checked: boolean) => void;
	extendSelection: (id: string) => boolean;
	startDrag: (id: string, event: React.PointerEvent<HTMLButtonElement>) => void;
	extendDrag: (
		id: string,
		event: React.PointerEvent<HTMLTableRowElement>,
	) => void;
};

const SelectionContext = createContext<SelectionContextValue | null>(null);
const SelectionRowContext = createContext<string | null>(null);

function useTableSelection() {
	const context = useContext(SelectionContext);
	if (!context) {
		throw new Error(
			"Selection components must be used within a SelectableTable",
		);
	}
	return context;
}

function useTableRowId() {
	const rowId = useContext(SelectionRowContext);
	if (rowId === null) {
		throw new Error(
			"SelectableTableCheckbox must be within a SelectableTableRow",
		);
	}
	return rowId;
}

function selectionsEqual(
	left: ReadonlySet<string>,
	right: ReadonlySet<string>,
) {
	return left.size === right.size && [...left].every((id) => right.has(id));
}

/**
 * Shift-click or Shift+Space applies the anchor checkbox's state to a range.
 * Shift-dragging toggles a range. Shrinking either restores selection outside it.
 */
export const SelectableTable: React.FC<SelectableTableProps> = ({
	rows,
	selectedIds,
	onSelectionChange,
	onClickCapture,
	children,
	...props
}) => {
	const rowsById = new Map(rows.map((row) => [row.id, row]));
	const anchorRef = useRef<SelectionAnchor | null>(null);
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

	const getRangeSelection = (
		anchorId: string,
		endId: string,
		checked: boolean,
		initialIds: ReadonlySet<string>,
	) => {
		const anchorIndex = rows.findIndex((row) => row.id === anchorId);
		const endIndex = rows.findIndex((row) => row.id === endId);
		const range = rows.slice(
			Math.min(anchorIndex, endIndex),
			Math.max(anchorIndex, endIndex) + 1,
		);
		const ids = new Set(initialIds);
		for (const row of range) {
			if (row.disabled) {
				continue;
			}
			if (checked) {
				ids.add(row.id);
			} else {
				ids.delete(row.id);
			}
		}
		return ids;
	};

	const setRowChecked: SelectionContextValue["setRowChecked"] = (
		id,
		checked,
	) => {
		anchorRef.current = { id, checked };
		const ids = new Set(selectedIds);
		if (checked) {
			ids.add(id);
		} else {
			ids.delete(id);
		}
		onSelectionChange(ids);
	};

	const extendSelection: SelectionContextValue["extendSelection"] = (id) => {
		const anchor = anchorRef.current;
		const range = anchor?.range;
		const row = anchor && rowsById.get(anchor.id);
		if (
			!anchor ||
			(anchor.id === id && !range) ||
			!row ||
			row.disabled ||
			selectedIds.has(anchor.id) !== anchor.checked ||
			(range && !selectionsEqual(selectedIds, range.expectedIds))
		) {
			return false;
		}
		const initialIds = range?.initialIds ?? new Set(selectedIds);
		const ids = getRangeSelection(anchor.id, id, anchor.checked, initialIds);
		anchor.range = { initialIds, expectedIds: ids };
		onSelectionChange(ids);
		return true;
	};

	const startDrag: SelectionContextValue["startDrag"] = (id, event) => {
		const row = rowsById.get(id);
		if (
			event.button !== 0 ||
			event.pointerType !== "mouse" ||
			!event.shiftKey ||
			event.ctrlKey ||
			!row ||
			row.disabled
		) {
			return;
		}
		// Preventing text selection also suppresses the browser's normal focus.
		event.preventDefault();
		event.currentTarget.focus({ preventScroll: true });
		cancelDrag();
		const initialIds = new Set(selectedIds);
		dragRef.current = {
			anchorId: id,
			pointerId: event.pointerId,
			checked: !selectedIds.has(id),
			initialIds,
			expectedIds: initialIds,
			renderedIds: selectedIds,
			moved: false,
			canceled: false,
		};
	};

	const extendDrag: SelectionContextValue["extendDrag"] = (id, event) => {
		const drag = dragRef.current;
		if (!drag || event.pointerId !== drag.pointerId || drag.canceled) {
			return;
		}
		if ((event.buttons & 1) === 0) {
			cancelDrag();
			return;
		}
		if (id !== drag.anchorId) {
			drag.moved = true;
		}
		const anchor = rowsById.get(drag.anchorId);
		if (
			!anchor ||
			anchor.disabled ||
			(selectedIds !== drag.renderedIds &&
				!selectionsEqual(selectedIds, drag.expectedIds))
		) {
			// Retain the gesture until pointerup to suppress its terminal click.
			drag.canceled = true;
			return;
		}
		if (!event.shiftKey || !drag.moved) {
			return;
		}

		const ids = getRangeSelection(
			drag.anchorId,
			id,
			drag.checked,
			drag.initialIds,
		);
		anchorRef.current = {
			id: drag.anchorId,
			checked: drag.checked,
			range: { initialIds: drag.initialIds, expectedIds: ids },
		};
		drag.renderedIds = selectedIds;
		drag.expectedIds = ids;
		onSelectionChange(ids);
	};

	return (
		<SelectionContext.Provider
			value={{
				rows: rowsById,
				selectedIds,
				onSelectionChange: (ids) => {
					anchorRef.current = null;
					cancelDrag();
					onSelectionChange(ids);
				},
				setRowChecked,
				extendSelection,
				startDrag,
				extendDrag,
			}}
		>
			<SelectionRowContext.Provider value={null}>
				<Table
					{...props}
					onClickCapture={(event) => {
						if (suppressClickRef.current && event.detail !== 0) {
							event.preventDefault();
							event.stopPropagation();
							cancelDrag();
							return;
						}
						onClickCapture?.(event);
					}}
				>
					{children}
				</Table>
			</SelectionRowContext.Provider>
		</SelectionContext.Provider>
	);
};

type SelectableTableRowProps = React.ComponentProps<typeof TableRow> & {
	rowId: string;
};

/** A row whose selection and drag behavior are owned by its SelectableTable. */
export const SelectableTableRow: React.FC<SelectableTableRowProps> = ({
	rowId,
	onPointerEnter,
	...props
}) => {
	const selection = useTableSelection();
	if (!selection.rows.has(rowId)) {
		throw new Error(
			"SelectableTableRow must reference a row in SelectableTable",
		);
	}
	const selected = selection.selectedIds.has(rowId);

	return (
		<SelectionRowContext.Provider value={rowId}>
			<TableRow
				{...props}
				aria-selected={selected}
				data-state={selected ? "selected" : undefined}
				onPointerEnter={(event) => {
					onPointerEnter?.(event);
					if (!event.defaultPrevented) {
						selection.extendDrag(rowId, event);
					}
				}}
			/>
		</SelectionRowContext.Provider>
	);
};

type SelectionCheckboxProps = Omit<
	React.ComponentProps<typeof Checkbox>,
	"checked" | "defaultChecked" | "onCheckedChange" | "disabled"
>;

/** A row checkbox that inherits identity and eligibility from its table. */
export const SelectableTableCheckbox: React.FC<SelectionCheckboxProps> = ({
	onClick,
	onKeyDown,
	onPointerDown,
	...props
}) => {
	const selection = useTableSelection();
	const rowId = useTableRowId();

	return (
		<Checkbox
			{...props}
			checked={selection.selectedIds.has(rowId)}
			disabled={selection.rows.get(rowId)?.disabled}
			onClick={(event) => {
				event.stopPropagation();
				onClick?.(event);
				if (
					!event.defaultPrevented &&
					event.shiftKey &&
					selection.extendSelection(rowId)
				) {
					event.preventDefault();
				}
			}}
			onKeyDown={(event) => {
				onKeyDown?.(event);
				if (
					!event.defaultPrevented &&
					event.key === " " &&
					event.shiftKey &&
					selection.extendSelection(rowId)
				) {
					event.preventDefault();
					event.stopPropagation();
				}
			}}
			onPointerDown={(event) => {
				onPointerDown?.(event);
				if (!event.defaultPrevented) {
					selection.startDrag(rowId, event);
				}
			}}
			onCheckedChange={(checked) => {
				selection.setRowChecked(rowId, checked === true);
			}}
		/>
	);
};

/**
 * Selects eligible rows, or clears all selection, including disabled or absent IDs.
 * An empty eligible set still permits clearing an existing selection.
 */
export const SelectableTableSelectAll: React.FC<SelectionCheckboxProps> = ({
	onClick,
	...props
}) => {
	const selection = useTableSelection();
	const eligibleRows = [...selection.rows.values()].filter(
		(row) => !row.disabled,
	);

	return (
		<Checkbox
			{...props}
			disabled={eligibleRows.length === 0 && selection.selectedIds.size === 0}
			checked={
				selection.selectedIds.size > 0 &&
				eligibleRows.every((row) => selection.selectedIds.has(row.id))
			}
			onClick={(event) => {
				event.stopPropagation();
				onClick?.(event);
			}}
			onCheckedChange={(checked) => {
				selection.onSelectionChange(
					new Set(checked ? eligibleRows.map((row) => row.id) : []),
				);
			}}
		/>
	);
};
