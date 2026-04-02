import {
	type RefObject,
	useCallback,
	useEffect,
	useLayoutEffect,
	useMemo,
	useRef,
	useState,
} from "react";

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
	const [overflowTabValues, setOverflowTabValues] = useState<string[]>([]);

	const recalculateOverflow = useCallback(() => {
		if (!enabled) {
			setOverflowTabValues([]);
			return;
		}

		const container = containerRef.current;
		if (!container) {
			return;
		}

		for (const tab of tabs) {
			const tabElement = container.querySelector<HTMLElement>(
				`[${DATA_ATTR_TAB_VALUE}="${tab.value}"]`,
			);
			if (tabElement) {
				tabWidthByValueRef.current[tab.value] = tabElement.offsetWidth;
			}
		}

		const alwaysVisibleTabs = tabs.slice(0, alwaysVisibleTabsCount);
		const optionalTabs = tabs.slice(alwaysVisibleTabsCount);
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
	}, [alwaysVisibleTabsCount, enabled, overflowTriggerWidthPx, tabs]);

	useLayoutEffect(() => {
		if (!isActive) {
			return;
		}
		recalculateOverflow();
	}, [isActive, recalculateOverflow]);

	useEffect(() => {
		if (!isActive) {
			return;
		}
		const container = containerRef.current;
		if (!container) {
			return;
		}
		const observer = new ResizeObserver(() => {
			recalculateOverflow();
		});
		observer.observe(container);
		return () => observer.disconnect();
	}, [isActive, recalculateOverflow]);

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

	const getTabMeasureProps = useCallback((tabValue: string) => {
		return { [DATA_ATTR_TAB_VALUE]: tabValue };
	}, []);

	return {
		containerRef,
		visibleTabs,
		overflowTabs,
		getTabMeasureProps,
	};
};
