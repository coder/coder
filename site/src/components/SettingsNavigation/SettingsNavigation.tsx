import { AnimatePresence, LayoutGroup, motion } from "motion/react";
import {
	type FC,
	type ReactNode,
	Suspense,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { flushSync } from "react-dom";
import { useLocation, useNavigationType } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { useMediaQuery } from "#/hooks/useMediaQuery";
import { belowLgViewportMediaQuery } from "#/utils/mobile";
import { SettingsNavigationBreadcrumb } from "./SettingsNavigationBreadcrumb";
import { SettingsNavigationSidebar } from "./SettingsNavigationSidebar";
import {
	findActiveSettingsNavigationItem,
	type LocatedSettingsNavigationItem,
	type SettingsNavigationItem,
	type SettingsNavigationSection,
} from "./types";

type SettingsNavigationProps = {
	title: string;
	sections: readonly SettingsNavigationSection[];
	storageKey: string;
	children: ReactNode;
	pageAdornment?: ReactNode;
};

const useSettingsNavigationCollapse = (storageKey: string) => {
	const isNarrow = useMediaQuery(belowLgViewportMediaQuery);
	const [preference, setPreference] = useState(
		() => localStorage.getItem(storageKey) === "true",
	);
	const [narrowExpansion, setNarrowExpansion] = useState({
		isNarrow,
		expanded: false,
	});
	if (narrowExpansion.isNarrow !== isNarrow) {
		setNarrowExpansion({ isNarrow, expanded: false });
	}
	const expandedWhileNarrow =
		narrowExpansion.isNarrow === isNarrow && narrowExpansion.expanded;
	const collapsed = isNarrow ? !expandedWhileNarrow : preference;
	const setCollapsed = (next: boolean) => {
		if (isNarrow) {
			setNarrowExpansion({ isNarrow, expanded: !next });
			return;
		}
		setPreference(next);
		localStorage.setItem(storageKey, String(next));
	};

	return { collapsed, setCollapsed };
};

const findOptimisticItem = (
	sections: readonly SettingsNavigationSection[],
	itemId: string,
): LocatedSettingsNavigationItem | undefined => {
	for (const section of sections) {
		const item = section.items.find((candidate) => candidate.id === itemId);
		if (item) {
			return { section, item };
		}
	}
	return undefined;
};

const transition = { duration: 0.2, ease: [0.4, 0, 0.2, 1] as const };

const MAX_SCROLL_POSITIONS = 100;
const scrollPositions = new Map<string, number>();

const scrollPositionKey = (storageKey: string, locationKey: string) =>
	`${storageKey}\0${locationKey}`;

const saveScrollPosition = (key: string, scrollTop: number) => {
	scrollPositions.delete(key);
	scrollPositions.set(key, scrollTop);
	if (scrollPositions.size <= MAX_SCROLL_POSITIONS) {
		return;
	}
	const oldestKey = scrollPositions.keys().next().value;
	if (oldestKey !== undefined) {
		scrollPositions.delete(oldestKey);
	}
};

const restoreScrollPosition = (key: string): number => {
	const scrollTop = scrollPositions.get(key) ?? 0;
	if (scrollPositions.has(key)) {
		scrollPositions.delete(key);
		scrollPositions.set(key, scrollTop);
	}
	return scrollTop;
};

export const SettingsNavigation: FC<SettingsNavigationProps> = ({
	title,
	sections,
	storageKey,
	children,
	pageAdornment,
}) => {
	const location = useLocation();
	const navigationType = useNavigationType();
	const located = findActiveSettingsNavigationItem(sections, location.pathname);
	const [optimisticSelection, setOptimisticSelection] = useState<{
		item: SettingsNavigationItem;
		fromPathname: string;
	}>();
	const active =
		optimisticSelection?.fromPathname === location.pathname
			? findOptimisticItem(sections, optimisticSelection.item.id)
			: located;
	const onSelect = (item: SettingsNavigationItem) =>
		setOptimisticSelection({ item, fromPathname: location.pathname });
	const { collapsed, setCollapsed } = useSettingsNavigationCollapse(storageKey);
	const [animateModeTransition, setAnimateModeTransition] = useState(false);
	const sidebarToggleRef = useRef<HTMLButtonElement>(null);
	const breadcrumbToggleRef = useRef<HTMLButtonElement>(null);
	const contentScrollRef = useRef<HTMLDivElement>(null);
	const toggle = (next: boolean) => {
		setAnimateModeTransition(true);
		flushSync(() => setCollapsed(next));
		const replacementToggle = next
			? breadcrumbToggleRef.current
			: sidebarToggleRef.current;
		replacementToggle?.focus();
	};
	const titleLayoutId = `${storageKey}-title`;
	const dividerLayoutId = `${storageKey}-divider`;

	useLayoutEffect(() => {
		document.documentElement.dataset.settingsNavigation = "";
		return () => {
			delete document.documentElement.dataset.settingsNavigation;
		};
	}, []);

	useLayoutEffect(() => {
		const scroller = contentScrollRef.current;
		if (!scroller) {
			return;
		}
		const positionKey = scrollPositionKey(storageKey, location.key);
		scroller.scrollTop =
			navigationType === "POP" ? restoreScrollPosition(positionKey) : 0;
		return () => {
			saveScrollPosition(positionKey, scroller.scrollTop);
		};
	}, [location.key, navigationType, storageKey]);

	return (
		<LayoutGroup id={storageKey}>
			<div className="flex h-full min-h-0 items-stretch overflow-hidden">
				<AnimatePresence initial={false} mode="popLayout">
					{!collapsed && (
						<motion.div
							key="sidebar"
							className="flex shrink-0"
							initial={{ opacity: 0 }}
							animate={{ opacity: 1 }}
							exit={{ opacity: 0 }}
							transition={transition}
						>
							<SettingsNavigationSidebar
								title={title}
								titleLayoutId={titleLayoutId}
								dividerLayoutId={dividerLayoutId}
								sections={sections}
								active={active}
								onCollapse={() => toggle(true)}
								onSelect={onSelect}
								animateControls={animateModeTransition}
								toggleRef={sidebarToggleRef}
							/>
						</motion.div>
					)}
				</AnimatePresence>
				<motion.div
					layout="position"
					transition={transition}
					className="flex min-h-0 min-w-0 grow flex-col"
				>
					{collapsed && (
						<SettingsNavigationBreadcrumb
							title={title}
							titleLayoutId={titleLayoutId}
							dividerLayoutId={dividerLayoutId}
							sections={sections}
							active={active}
							pageAdornment={pageAdornment}
							onExpand={() => toggle(false)}
							onSelect={onSelect}
							animateControls={animateModeTransition}
							toggleRef={breadcrumbToggleRef}
						/>
					)}
					<div
						ref={contentScrollRef}
						role="region"
						aria-label={`${title} content`}
						className="relative flex min-h-0 flex-1 flex-col overflow-x-hidden overflow-y-auto overscroll-contain"
					>
						<section className="w-full max-w-(--breakpoint-2xl) px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
							<Suspense fallback={<Loader />}>{children}</Suspense>
						</section>
					</div>
				</motion.div>
			</div>
		</LayoutGroup>
	);
};

export type { SettingsNavigationSection } from "./types";
