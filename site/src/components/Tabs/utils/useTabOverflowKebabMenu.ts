import { type RefObject, useEffect, useMemo, useRef, useState } from "react";

type TabLike = {
	value: string;
};

type UseTabOverflowKebabMenuOptions<TTab extends TabLike> = {
	tabs: readonly TTab[];
	enabled: boolean;
	isActive: boolean;
	alwaysVisibleTabsCount?: number;
	overflowTriggerWidthPx?: number;
};

type UseTabOverflowKebabMenuResult<TTab extends TabLike> = {
	containerRef: RefObject<HTMLDivElement | null>;
	visibleTabs: TTab[];
	overflowTabs: TTab[];
	getTabMeasureProps: (tabValue: string) => Record<string, string>;
};

const DATA_ATTR_TAB_VALUE = "data-tab-overflow-item-value";

export const useTabOverflowKebabMenu = <TTab extends TabLike>({
	tabs,
	enabled,
	isActive,
	alwaysVisibleTabsCount = 1,
	overflowTriggerWidthPx = 44,
}: UseTabOverflowKebabMenuOptions<TTab>): UseTabOverflowKebabMenuResult<TTab> => {
	const containerRef = useRef<HTMLDivElement>(null);
	const tabWidthByValueRef = useRef<Record<string, number>>({});
	const tabsRef = useRef(tabs);
	const [overflowTabValues, setOverflowTabValues] = useState<string[]>([]);
	tabsRef.current = tabs;

	useEffect(() => {
		if (!enabled || !isActive) {
			setOverflowTabValues([]);
			return;
		}

		const container = containerRef.current;
		if (!container) {
			return;
		}

		const recalculateOverflow = () => {
			const currentTabs = tabsRef.current;
			for (const tab of currentTabs) {
				const tabElement = container.querySelector<HTMLElement>(
					`[${DATA_ATTR_TAB_VALUE}="${tab.value}"]`,
				);
				if (tabElement) {
					tabWidthByValueRef.current[tab.value] = tabElement.offsetWidth;
				}
			}

			const alwaysVisibleTabs = currentTabs.slice(0, alwaysVisibleTabsCount);
			const optionalTabs = currentTabs.slice(alwaysVisibleTabsCount);
			if (optionalTabs.length === 0) {
				setOverflowTabValues([]);
				return;
			}

			const alwaysVisibleWidth = alwaysVisibleTabs.reduce((total, tab) => {
				return total + (tabWidthByValueRef.current[tab.value] ?? 0);
			}, 0);

			const availableWidth = container.clientWidth;
			let usedWidth = alwaysVisibleWidth;
			const nextOverflowValues: string[] = [];

			for (let i = 0; i < optionalTabs.length; i++) {
				const tab = optionalTabs[i];
				const tabWidth = tabWidthByValueRef.current[tab.value] ?? 0;
				const hasMoreTabsAfterCurrent = i < optionalTabs.length - 1;
				const widthNeeded =
					usedWidth +
					tabWidth +
					(hasMoreTabsAfterCurrent ? overflowTriggerWidthPx : 0);

				if (widthNeeded <= availableWidth) {
					usedWidth += tabWidth;
					continue;
				}

				nextOverflowValues.push(
					...optionalTabs.slice(i).map((overflowTab) => overflowTab.value),
				);
				break;
			}

			setOverflowTabValues((currentValues) => {
				if (
					currentValues.length === nextOverflowValues.length &&
					currentValues.every(
						(value, index) => value === nextOverflowValues[index],
					)
				) {
					return currentValues;
				}
				return nextOverflowValues;
			});
		};

		recalculateOverflow();
		const observer = new ResizeObserver(recalculateOverflow);
		observer.observe(container);
		return () => observer.disconnect();
	}, [alwaysVisibleTabsCount, enabled, isActive, overflowTriggerWidthPx]);

	const overflowTabValuesSet = useMemo(
		() => new Set(overflowTabValues),
		[overflowTabValues],
	);

	const visibleTabs = useMemo(
		() => tabs.filter((tab) => !overflowTabValuesSet.has(tab.value)),
		[tabs, overflowTabValuesSet],
	);
	const overflowTabs = useMemo(
		() => tabs.filter((tab) => overflowTabValuesSet.has(tab.value)),
		[tabs, overflowTabValuesSet],
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
