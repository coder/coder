import {
	type RefObject,
	useCallback,
	useLayoutEffect,
	useRef,
	useState,
} from "react";

type TabValue = {
	value: string;
};

type UseKebabMenuOptions<T extends TabValue> = {
	tabs: readonly T[];
	enabled: boolean;
	isActive: boolean;
	overflowTriggerWidth?: number;
};

type UseKebabMenuResult<T extends TabValue> = {
	containerRef: RefObject<HTMLDivElement | null>;
	visibleTabs: T[];
	overflowTabs: T[];
	getTabMeasureProps: (tabValue: string) => Record<string, string>;
};

const ALWAYS_VISIBLE_TABS_COUNT = 1;
const DATA_ATTR_TAB_VALUE = "data-tab-overflow-item-value";

/**
 * Splits tabs into visible and overflow groups based on container width.
 *
 * Tabs must render with `getTabMeasureProps()` so this hook can measure
 * trigger widths from the DOM.
 */
export const useKebabMenu = <T extends TabValue>({
	tabs,
	enabled,
	isActive,
	overflowTriggerWidth = 44,
}: UseKebabMenuOptions<T>): UseKebabMenuResult<T> => {
	const containerRef = useRef<HTMLDivElement>(null);
	// Width cache prevents oscillation when overflow tabs are not mounted.
	const tabWidthByValueRef = useRef<Record<string, number>>({});
	const [overflowTabValues, setTabValues] = useState<string[]>([]);

	const recalculateOverflow = useCallback(
		(availableWidth: number) => {
			if (!enabled || !isActive) {
				// Keep this update idempotent to avoid render loops.
				setTabValues((currentValues) => {
					if (currentValues.length === 0) {
						return currentValues;
					}
					return [];
				});
				return;
			}

			const container = containerRef.current;
			if (!container) {
				return;
			}
			const tabWidthByValue = measureTabWidths({
				tabs,
				container,
				previousTabWidthByValue: tabWidthByValueRef.current,
			});
			tabWidthByValueRef.current = tabWidthByValue;
			const tabGap = getTabGap(container);

			const nextOverflowValues = calculateTabValues({
				tabs,
				availableWidth,
				tabWidthByValue,
				overflowTriggerWidth,
				tabGap,
			});

			setTabValues((currentValues) => {
				// Avoid state updates when the computed overflow did not change.
				if (areStringArraysEqual(currentValues, nextOverflowValues)) {
					return currentValues;
				}
				return nextOverflowValues;
			});
		},
		[enabled, isActive, overflowTriggerWidth, tabs],
	);

	useLayoutEffect(() => {
		const container = containerRef.current;
		if (!enabled || !isActive) {
			// Keep this update idempotent to avoid render loops.
			setTabValues((currentValues) => {
				if (currentValues.length === 0) {
					return currentValues;
				}
				return [];
			});
			return;
		}
		if (!container) {
			return;
		}

		recalculateOverflow(getContentBoxWidth(container));

		// Recompute whenever ResizeObserver reports a container width change.
		const observer = new ResizeObserver(([entry]) => {
			if (!entry) {
				return;
			}
			const nextAvailableWidth = Math.max(0, entry.contentRect.width);
			recalculateOverflow(nextAvailableWidth);
		});
		observer.observe(container);
		return () => observer.disconnect();
	}, [recalculateOverflow, enabled, isActive]);

	const overflowTabValuesSet = new Set(overflowTabValues);
	const { visibleTabs, overflowTabs } = tabs.reduce<{
		visibleTabs: T[];
		overflowTabs: T[];
	}>(
		(tabGroups, tab) => {
			if (overflowTabValuesSet.has(tab.value)) {
				tabGroups.overflowTabs.push(tab);
			} else {
				tabGroups.visibleTabs.push(tab);
			}
			return tabGroups;
		},
		{ visibleTabs: [], overflowTabs: [] },
	);

	const getTabMeasureProps = (tabValue: string) => {
		return { [DATA_ATTR_TAB_VALUE]: tabValue };
	};

	return {
		containerRef,
		visibleTabs,
		overflowTabs,
		getTabMeasureProps,
	};
};

const calculateTabValues = <T extends TabValue>({
	tabs,
	availableWidth,
	tabWidthByValue,
	overflowTriggerWidth,
	tabGap,
}: {
	tabs: readonly T[];
	availableWidth: number;
	tabWidthByValue: Readonly<Record<string, number>>;
	overflowTriggerWidth: number;
	tabGap: number;
}): string[] => {
	if (tabs.length <= ALWAYS_VISIBLE_TABS_COUNT) {
		return [];
	}

	let usedWidth = 0;
	let visibleCount = 0;

	for (const [index, tab] of tabs.entries()) {
		const tabWidth = tabWidthByValue[tab.value] ?? 0;
		const gapBeforeTab = visibleCount > 0 ? tabGap : 0;
		const usedWidthWithTab = usedWidth + gapBeforeTab + tabWidth;
		const hasMoreTabs = index < tabs.length - 1;
		// Reserve kebab trigger width whenever additional tabs remain.
		const widthNeeded =
			usedWidthWithTab + (hasMoreTabs ? tabGap + overflowTriggerWidth : 0);

		if (index < ALWAYS_VISIBLE_TABS_COUNT || widthNeeded <= availableWidth) {
			usedWidth = usedWidthWithTab;
			visibleCount += 1;
			continue;
		}

		return tabs.slice(index).map((overflowTab) => overflowTab.value);
	}

	return [];
};

const measureTabWidths = <T extends TabValue>({
	tabs,
	container,
	previousTabWidthByValue,
}: {
	tabs: readonly T[];
	container: HTMLDivElement;
	previousTabWidthByValue: Readonly<Record<string, number>>;
}): Record<string, number> => {
	const nextTabWidthByValue = { ...previousTabWidthByValue };
	for (const tab of tabs) {
		const tabElement = container.querySelector<HTMLElement>(
			`[${DATA_ATTR_TAB_VALUE}="${tab.value}"]`,
		);
		if (tabElement) {
			nextTabWidthByValue[tab.value] = tabElement.offsetWidth;
		}
	}
	return nextTabWidthByValue;
};

const getContentBoxWidth = (container: HTMLElement): number => {
	const styles = window.getComputedStyle(container);
	const paddingLeft = Number.parseFloat(styles.paddingLeft) || 0;
	const paddingRight = Number.parseFloat(styles.paddingRight) || 0;
	return container.clientWidth - paddingLeft - paddingRight;
};

const getTabGap = (container: HTMLElement): number => {
	const styles = window.getComputedStyle(container);
	const gap = Number.parseFloat(styles.columnGap);
	return Number.isFinite(gap) ? gap : 0;
};

const areStringArraysEqual = (
	left: readonly string[],
	right: readonly string[],
): boolean => {
	return (
		left.length === right.length &&
		left.every((value, index) => value === right[index])
	);
};
