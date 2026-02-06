import { useRef } from "react";

/**
 * Result from the shift-click handler.
 * If shift was held, returns the indices to toggle.
 * If not, returns null (handle as normal single selection).
 */
export interface ShiftClickResult {
	/** Indices of items that should be toggled */
	indicesToToggle: number[];
	/** Whether to select or deselect the items */
	shouldSelect: boolean;
}

interface UseRowRangeSelectionReturn {
	/**
	 * Handle a checkbox click. Call this in onClick before your normal selection logic.
	 *
	 * @param event - The mouse event
	 * @param index - The index of the clicked item in the array
	 * @param isCurrentlySelected - Whether the clicked item is currently selected
	 * @param totalItems - Total number of items in the list
	 * @returns ShiftClickResult if shift was held, null otherwise
	 */
	handleClick: (
		event: React.MouseEvent<HTMLButtonElement>,
		index: number,
		isCurrentlySelected: boolean,
		totalItems: number,
	) => ShiftClickResult | null;
}

/**
 * Hook to enable shift+click range selection for table rows.
 *
 * Usage:
 * ```tsx
 * const { handleClick } = useRowRangeSelection();
 *
 * // In your checkbox onClick:
 * onClick={(e) => {
 *   e.stopPropagation();
 *   const result = handleClick(e, index, isSelected, items.length);
 *   if (result) {
 *     // Shift was held - toggle all items in range
 *     for (const i of result.indicesToToggle) {
 *       toggleItem(items[i], result.shouldSelect);
 *     }
 *   }
 *   // If null, let onCheckedChange handle normal single-item toggle
 * }}
 * ```
 */
export function useRowRangeSelection(): UseRowRangeSelectionReturn {
	const lastSelectedIndexRef = useRef<number>(-1);

	const handleClick = (
		event: React.MouseEvent<HTMLButtonElement>,
		index: number,
		isCurrentlySelected: boolean,
		totalItems: number,
	): ShiftClickResult | null => {
		const previousIndex = lastSelectedIndexRef.current;
		lastSelectedIndexRef.current = index;

		// Check if shift was held and we have a valid previous selection
		if (
			event.shiftKey &&
			previousIndex >= 0 &&
			previousIndex < totalItems &&
			previousIndex !== index
		) {
			// Calculate the range (inclusive of both ends)
			const start = Math.min(previousIndex, index);
			const end = Math.max(previousIndex, index);

			const indicesToToggle: number[] = [];
			for (let i = start; i <= end; i++) {
				indicesToToggle.push(i);
			}

			return {
				indicesToToggle,
				// If the clicked item is selected, we're deselecting the range
				// If the clicked item is not selected, we're selecting the range
				shouldSelect: !isCurrentlySelected,
			};
		}

		return null;
	};

	return { handleClick };
}
