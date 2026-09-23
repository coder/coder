import { cn } from "cn";
import { PanelLeftIcon, PanelTopIcon } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { type FC, useLayoutEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { Button } from "#/components/Button/Button";
import type {
	LocatedSettingsNavigationItem,
	SettingsNavigationItem,
	SettingsNavigationSection,
} from "./types";
import { shouldUseOptimisticSelection } from "./types";

type NavigationToggleProps = {
	collapsed: boolean;
	onClick: () => void;
	animateIn: boolean;
	toggleRef: React.Ref<HTMLButtonElement>;
};

export const NavigationToggle: FC<NavigationToggleProps> = ({
	collapsed,
	onClick,
	animateIn,
	toggleRef,
}) => (
	<motion.div
		className="shrink-0"
		initial={animateIn ? { x: collapsed ? -8 : 8, opacity: 0 } : false}
		animate={{ x: 0, opacity: 1 }}
		transition={{ duration: 0.14, ease: "easeOut", delay: 0.1 }}
	>
		<Button
			ref={toggleRef}
			variant="subtle"
			size="icon"
			onClick={onClick}
			aria-label={collapsed ? "Expand navigation" : "Collapse navigation"}
		>
			{collapsed ? <PanelLeftIcon /> : <PanelTopIcon />}
		</Button>
	</motion.div>
);

type NavigationTitleProps = {
	title: string;
	layoutId: string;
	collapsed?: boolean;
};

export const NavigationTitle: FC<NavigationTitleProps> = ({
	title,
	layoutId,
	collapsed,
}) => (
	<motion.span
		layoutId={layoutId}
		layout="position"
		initial={false}
		animate={{
			color: collapsed
				? "var(--color-content-secondary)"
				: "var(--color-content-primary)",
		}}
		className="truncate px-1 text-sm font-medium"
	>
		{title}
	</motion.span>
);

type IndicatorRect = { top: number; height: number };

type IndicatorPosition = {
	from?: IndicatorRect;
	to: IndicatorRect;
};

const travelKeyframes = (from: IndicatorRect, to: IndicatorRect) => {
	const start = Math.min(from.top, to.top);
	const end = Math.max(from.top + from.height, to.top + to.height);
	return {
		top: [from.top, start, to.top],
		height: [from.height, end - start, to.height],
		width: [3, 4, 3],
		scaleY: 1,
	};
};

const ActiveIndicator: FC<{ target: HTMLElement | null }> = ({ target }) => {
	const [position, setPosition] = useState<IndicatorPosition>();

	useLayoutEffect(() => {
		if (!target) {
			setPosition(undefined);
			return;
		}
		const measure = () => {
			const next = { top: target.offsetTop, height: target.offsetHeight };
			setPosition((current) => {
				if (current?.to.top === next.top && current.to.height === next.height) {
					return current;
				}
				return { from: current?.to, to: next };
			});
		};
		measure();
		const observer = new ResizeObserver(measure);
		observer.observe(target);
		return () => observer.disconnect();
	}, [target]);

	return (
		<AnimatePresence>
			{target && position && (
				<motion.div
					aria-hidden
					className="absolute -left-px origin-center rounded-full bg-content-primary"
					initial={{
						top: position.from?.top ?? position.to.top,
						height: position.from?.height ?? position.to.height,
						width: 3,
						scaleY: position.from ? 1 : 0,
					}}
					animate={
						position.from
							? travelKeyframes(position.from, position.to)
							: {
									top: position.to.top,
									height: position.to.height,
									width: 3,
									scaleY: 1,
								}
					}
					exit={{ scaleY: 0, transition: { duration: 0.12, ease: "easeIn" } }}
					transition={
						position.from
							? {
									duration: 0.5,
									times: [0, 0.45, 1],
									ease: ["easeIn", [0.2, 1.4, 0.4, 1]],
								}
							: { type: "spring", stiffness: 380, damping: 22 }
					}
				/>
			)}
		</AnimatePresence>
	);
};

type SidebarSectionProps = {
	section: SettingsNavigationSection;
	activeItemId: string | undefined;
	onSelect: (item: SettingsNavigationItem) => void;
};

const SidebarSection: FC<SidebarSectionProps> = ({
	section,
	activeItemId,
	onSelect,
}) => {
	const [activeElement, setActiveElement] = useState<HTMLElement | null>(null);
	const linkRefs = useRef(new Map<string, HTMLAnchorElement>());

	useLayoutEffect(() => {
		setActiveElement(
			activeItemId ? (linkRefs.current.get(activeItemId) ?? null) : null,
		);
	}, [activeItemId]);

	const links = (
		<ul className="m-0 flex list-none flex-col p-0">
			{section.items.map((item) => (
				<li key={item.id}>
					<Link
						ref={(element) => {
							if (element) {
								linkRefs.current.set(item.id, element);
							} else {
								linkRefs.current.delete(item.id);
							}
						}}
						to={item.href}
						onClick={(event) => {
							if (shouldUseOptimisticSelection(event)) {
								onSelect(item);
							}
						}}
						aria-current={item.id === activeItemId ? "page" : undefined}
						className={cn(
							"flex h-8 items-center px-4 text-sm font-medium no-underline transition-colors",
							item.id === activeItemId
								? "text-content-primary"
								: "text-content-secondary hover:text-content-primary",
						)}
					>
						{item.label}
					</Link>
				</li>
			))}
		</ul>
	);

	if (!section.label) {
		return (
			<div className="relative">
				<ActiveIndicator target={activeElement} />
				{links}
			</div>
		);
	}

	return (
		<div className="flex flex-col gap-2">
			<div className="text-sm font-medium text-content-secondary opacity-70">
				{section.label}
			</div>
			<div className="relative border-0 border-l border-solid border-border">
				<ActiveIndicator target={activeElement} />
				{links}
			</div>
		</div>
	);
};

type SettingsNavigationSidebarProps = {
	title: string;
	titleLayoutId: string;
	dividerLayoutId: string;
	sections: readonly SettingsNavigationSection[];
	active: LocatedSettingsNavigationItem | undefined;
	onCollapse: () => void;
	onSelect: (item: SettingsNavigationItem) => void;
	animateControls: boolean;
	toggleRef: React.Ref<HTMLButtonElement>;
};

export const SettingsNavigationSidebar: FC<SettingsNavigationSidebarProps> = ({
	title,
	titleLayoutId,
	dividerLayoutId,
	sections,
	active,
	onCollapse,
	onSelect,
	animateControls,
	toggleRef,
}) => (
	<aside className="flex h-full w-60 shrink-0 flex-col border-0 border-r border-solid border-border">
		<div className="flex h-14 shrink-0 items-center justify-between gap-2 px-5">
			<NavigationTitle title={title} layoutId={titleLayoutId} />
			<NavigationToggle
				collapsed={false}
				onClick={onCollapse}
				animateIn={animateControls}
				toggleRef={toggleRef}
			/>
		</div>
		<motion.div
			layoutId={dividerLayoutId}
			className="h-px shrink-0 bg-border"
			transition={{ layout: { duration: 0.12, ease: "easeOut" } }}
		/>
		<nav
			aria-label={title}
			className={cn(
				"min-h-0 flex-1 overflow-y-auto px-6 py-6",
				sections.some((section) => section.label)
					? "flex flex-col gap-6"
					: "block",
			)}
		>
			{sections.map((section) => (
				<SidebarSection
					key={section.id}
					section={section}
					activeItemId={
						active?.section.id === section.id ? active.item.id : undefined
					}
					onSelect={onSelect}
				/>
			))}
		</nav>
	</aside>
);
