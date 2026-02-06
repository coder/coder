import type { Row, Table } from "@tanstack/react-table";
import { useRef } from "react";

/**
 * Find all rows between two row IDs (inclusive), supporting paginated and
 * grouped data. Based on the implementation from:
 * https://github.com/TanStack/table/discussions/3068#discussioncomment-12329944
 */
function getRowRange<TData>(
	rows: Array<Row<TData>>,
	clickedRowID: string,
	previousClickedRowID: string,
): Array<Row<TData>> {
	const range: Array<Row<TData>> = [];
	const processedRowsMap: Record<string, boolean> = {
		[clickedRowID]: false,
		[previousClickedRowID]: false,
	};

	for (const row of rows) {
		if (row.id === clickedRowID || row.id === previousClickedRowID) {
			if (previousClickedRowID === "") {
				range.push(row);
				break;
			}
			processedRowsMap[row.id] = true;
		}

		if (
			(processedRowsMap[clickedRowID] ||
				processedRowsMap[previousClickedRowID]) &&
			!row.getIsGrouped()
		) {
			range.push(row);
		}

		if (
			processedRowsMap[clickedRowID] &&
			processedRowsMap[previousClickedRowID]
		) {
			break;
		}
	}

	return range;
}

interface UseRowRangeSelectionReturn<TData> {
	/**
	 * Handle a checkbox click event. Call this in the onClick handler of your
	 * row checkbox. Returns the rows that should be toggled if shift was held,
	 * or null if this was a normal click.
	 *
	 * @returns Array of rows to toggle if shift+click, null otherwise
	 */
	handleShiftClick: (
		event: React.MouseEvent<HTMLButtonElement>,
		row: Row<TData>,
		table: Table<TData>,
	) => Array<Row<TData>> | null;
}

/**
 * Hook to enable shift+click range selection for table rows.
 *
 * This hook works with external selection state (not tanstack's built-in
 * row selection). It returns the rows that should be toggled when shift+click
 * is detected, allowing the calling code to update its own selection state.
 *
 * Usage:
 * ```tsx
 * const { handleShiftClick } = useRowRangeSelection<MyData>();
 *
 * // In your checkbox onClick:
 * onClick={(e) => {
 *   e.stopPropagation();
 *   const rowsToToggle = handleShiftClick(e, row, table);
 *   if (rowsToToggle) {
 *     // Shift was held - update selection for all rows in range
 *     const newSelection = new Set(currentSelection);
 *     const shouldSelect = !currentSelection.has(row.id);
 *     for (const r of rowsToToggle) {
 *       if (shouldSelect) newSelection.add(r.id);
 *       else newSelection.delete(r.id);
 *     }
 *     onSelectionChange(newSelection);
 *   }
 *   // If null, let onCheckedChange handle normal single-row toggle
 * }}
 * ```
 */
export function useRowRangeSelection<
	TData,
>(): UseRowRangeSelectionReturn<TData> {
	const lastSelectedRowIdRef = useRef<string>("");

	const handleShiftClick = (
		event: React.MouseEvent<HTMLButtonElement>,
		row: Row<TData>,
		table: Table<TData>,
	): Array<Row<TData>> | null => {
		const previousId = lastSelectedRowIdRef.current;
		lastSelectedRowIdRef.current = row.id;

		if (event.shiftKey && previousId !== "") {
			const { rows } = table.getRowModel();
			const rowIds = rows.map((r) => r.id);

			// Only do range selection if the previous row is still visible
			if (rowIds.includes(previousId)) {
				return getRowRange(rows, row.id, previousId);
			}
		}

		return null;
	};

	return { handleShiftClick };
}
