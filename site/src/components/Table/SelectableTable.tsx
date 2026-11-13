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

type DragState = {
	anchorId: string;
	pointerId: number;
	select: boolean;
	initialIds: ReadonlySet<string>;
	moved: boolean;
};

type SelectionContextValue = {
	rows: ReadonlyMap<string, SelectableRow>;
	selectedIds: ReadonlySet<string>;
	onSelectionChange: (selectedIds: Set<string>) => void;
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

/**
 * A controlled table composed with SelectableTableRow and SelectableTableCheckbox.
 * Dragging from a selected checkbox deselects; reversing restores selection
 * outside the range.
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

	const startDrag: SelectionContextValue["startDrag"] = (id, event) => {
		const row = rowsById.get(id);
		if (
			event.button !== 0 ||
			event.pointerType !== "mouse" ||
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
		dragRef.current = {
			anchorId: id,
			pointerId: event.pointerId,
			select: !selectedIds.has(id),
			initialIds: new Set(selectedIds),
			moved: false,
		};
	};

	const extendDrag: SelectionContextValue["extendDrag"] = (id, event) => {
		const drag = dragRef.current;
		if (!drag || event.pointerId !== drag.pointerId) {
			return;
		}
		if ((event.buttons & 1) === 0 || !rowsById.has(drag.anchorId)) {
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
		for (const row of rows) {
			if (endpoints.delete(row.id)) {
				inRange = true;
			}
			if (inRange && !row.disabled) {
				if (drag.select) {
					ids.add(row.id);
				} else {
					ids.delete(row.id);
				}
			}
			if (endpoints.size === 0) {
				break;
			}
		}
		onSelectionChange(ids);
	};

	return (
		<SelectionContext.Provider
			value={{
				rows: rowsById,
				selectedIds,
				onSelectionChange,
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
	onPointerDown,
	...props
}) => {
	const selection = useTableSelection();
	const rowId = useContext(SelectionRowContext);
	if (rowId === null) {
		throw new Error(
			"SelectableTableCheckbox must be within a SelectableTableRow",
		);
	}

	return (
		<Checkbox
			{...props}
			checked={selection.selectedIds.has(rowId)}
			disabled={selection.rows.get(rowId)?.disabled}
			onClick={(event) => {
				event.stopPropagation();
				onClick?.(event);
			}}
			onPointerDown={(event) => {
				onPointerDown?.(event);
				if (!event.defaultPrevented) {
					selection.startDrag(rowId, event);
				}
			}}
			onCheckedChange={(checked) => {
				const ids = new Set(selection.selectedIds);
				if (checked) {
					ids.add(rowId);
				} else {
					ids.delete(rowId);
				}
				selection.onSelectionChange(ids);
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
