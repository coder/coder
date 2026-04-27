import {
	type RefObject,
	useCallback,
	useEffect,
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
	const tabsRef = useRef<readonly T[]>(tabs);
	tabsRef.current = tabs;
	const previousTabsRef = useRef<readonly T[]>(tabs);
	const availableWidthRef = useRef<number | null>(null);
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
			const currentTabs = tabsRef.current;

			const tabWidthByValue = measureTabWidths({
				tabs: currentTabs,
				container,
				previousTabWidthByValue: tabWidthByValueRef.current,
			});
			tabWidthByValueRef.current = tabWidthByValue;

			const nextOverflowValues = calculateTabValues({
				tabs: currentTabs,
				availableWidth,
				tabWidthByValue,
				overflowTriggerWidth,
			});

			setTabValues((currentValues) => {
				// Avoid state updates when the computed overflow did not change.
				if (areStringArraysEqual(currentValues, nextOverflowValues)) {
					return currentValues;
				}
				return nextOverflowValues;
			});
		},
		[enabled, isActive, overflowTriggerWidth],
	);

	useEffect(() => {
		if (previousTabsRef.current === tabs) {
			// No change in tabs, no need to recalculate.
			return;
		}
		previousTabsRef.current = tabs;
		if (availableWidthRef.current === null) {
			// First mount, no width available yet.
			return;
		}
		recalculateOverflow(availableWidthRef.current);
	}, [recalculateOverflow, tabs]);

	useLayoutEffect(() => {
		const container = containerRef.current;
		if (!container || !enabled || !isActive) {
			return;
		}

		// Recompute whenever ResizeObserver reports a container width change.
		const observer = new ResizeObserver(([entry]) => {
			if (!entry) {
				return;
			}
			availableWidthRef.current = entry.contentRect.width;
			recalculateOverflow(entry.contentRect.width);
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
}: {
	tabs: readonly T[];
	availableWidth: number;
	tabWidthByValue: Readonly<Record<string, number>>;
	overflowTriggerWidth: number;
}): string[] => {
	const tabWidthByValueMap = new Map<string, number>();
	for (const tab of tabs) {
		tabWidthByValueMap.set(tab.value, tabWidthByValue[tab.value] ?? 0);
	}

	const firstOptionalTabIndex = Math.min(
		ALWAYS_VISIBLE_TABS_COUNT,
		tabs.length,
	);
	if (firstOptionalTabIndex >= tabs.length) {
		return [];
	}

	const alwaysVisibleTabs = tabs.slice(0, firstOptionalTabIndex);
	const optionalTabs = tabs.slice(firstOptionalTabIndex);
	const alwaysVisibleWidth = alwaysVisibleTabs.reduce((total, tab) => {
		return total + (tabWidthByValueMap.get(tab.value) ?? 0);
	}, 0);
	const firstTabIndex = findFirstTabIndex({
		optionalTabs,
		optionalTabWidths: optionalTabs.map((tab) => {
			return tabWidthByValueMap.get(tab.value) ?? 0;
		}),
		startingUsedWidth: alwaysVisibleWidth,
		availableWidth,
		overflowTriggerWidth,
	});

	if (firstTabIndex === -1) {
		return [];
	}

	return optionalTabs
		.slice(firstTabIndex)
		.map((overflowTab) => overflowTab.value);
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

const findFirstTabIndex = ({
	optionalTabs,
	optionalTabWidths,
	startingUsedWidth,
	availableWidth,
	overflowTriggerWidth,
}: {
	optionalTabs: readonly TabValue[];
	optionalTabWidths: readonly number[];
	startingUsedWidth: number;
	availableWidth: number;
	overflowTriggerWidth: number;
}): number => {
	const result = optionalTabs.reduce(
		(acc, _tab, index) => {
			if (acc.firstTabIndex !== -1) {
				return acc;
			}

			const tabWidth = optionalTabWidths[index] ?? 0;
			const hasMoreTabs = index < optionalTabs.length - 1;
			// Reserve kebab trigger width whenever additional tabs remain.
			const widthNeeded =
				acc.usedWidth + tabWidth + (hasMoreTabs ? overflowTriggerWidth : 0);

			if (widthNeeded <= availableWidth) {
				return {
					usedWidth: acc.usedWidth + tabWidth,
					firstTabIndex: -1,
				};
			}

			return {
				usedWidth: acc.usedWidth,
				firstTabIndex: index,
			};
		},
		{ usedWidth: startingUsedWidth, firstTabIndex: -1 },
	);

	return result.firstTabIndex;
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
