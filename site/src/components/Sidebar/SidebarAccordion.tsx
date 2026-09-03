import type { ElementType, FC, ReactNode } from "react";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { useSidebarContext } from "./SidebarContext";

interface SidebarAccordionProps {
	/**
	 * Icon shown before the label. Top-level accordions need one so the
	 * collapsed icon rail can represent them; nested accordions omit it.
	 */
	icon?: ElementType;
	label: string;
	children: ReactNode;
	open: boolean;
	onToggle: () => void;
	/** Whether this section contains the current route. */
	active?: boolean;
}

/**
 * Expand/collapse section header for the settings sidebars. The header
 * only toggles the section and never navigates. Children render in an
 * indented list with a vertical connecting line, and accordions nest
 * so each level adds another indented line.
 *
 * When the sidebar is collapsed to its icon rail, an accordion with an
 * icon renders as that icon with a tooltip; clicking it expands the
 * sidebar and opens the section. Accordions without an icon are only
 * reachable inside an open parent, so they never render collapsed.
 */
export const SidebarAccordion: FC<SidebarAccordionProps> = ({
	icon: Icon,
	label,
	children,
	open,
	onToggle,
	active = false,
}) => {
	const { collapsed, expand } = useSidebarContext();

	// Icon and label highlight when this section owns the current
	// route, regardless of whether the accordion is expanded.
	const iconClass = cn(
		"size-4 flex-shrink-0 text-content-secondary",
		active && "text-content-primary",
	);
	const labelClass = cn(
		"text-sm font-medium text-content-secondary whitespace-nowrap",
		active && "text-content-primary",
	);

	if (collapsed && Icon) {
		return (
			<TooltipProvider>
				<Tooltip delayDuration={0}>
					<TooltipTrigger asChild>
						<button
							type="button"
							onClick={() => {
								expand();
								if (!open) {
									onToggle();
								}
							}}
							className="flex items-center justify-center w-10 h-10 rounded-md cursor-pointer bg-transparent border-none hover:bg-surface-secondary"
						>
							<Icon className={iconClass} />
						</button>
					</TooltipTrigger>
					<TooltipContent side="right">{label}</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}

	return (
		<Collapsible open={open} onOpenChange={onToggle}>
			<CollapsibleTrigger asChild>
				<button
					type="button"
					className="flex w-full items-center gap-2 px-3 py-2 h-10 rounded-md cursor-pointer bg-transparent border-none hover:bg-surface-secondary transition-colors"
				>
					{Icon && <Icon className={iconClass} />}
					<span className={labelClass}>{label}</span>
					<ChevronDownIcon
						open={open}
						className="size-4 text-content-secondary ml-auto flex-shrink-0"
					/>
				</button>
			</CollapsibleTrigger>
			<CollapsibleContent>
				{/* Children of an icon section align under its label. Nested
				    sections draw a connecting line at the label's left edge,
				    with their sub-items indented past it. */}
				<div
					className={cn(
						"flex flex-col gap-1",
						Icon
							? "ml-6"
							: "ml-3 pl-1 border-0 border-l border-solid border-border",
					)}
				>
					{children}
				</div>
			</CollapsibleContent>
		</Collapsible>
	);
};
