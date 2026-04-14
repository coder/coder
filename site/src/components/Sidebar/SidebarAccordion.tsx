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
	icon: ElementType;
	label: string;
	children: ReactNode;
	open: boolean;
	onToggle: () => void;
}

/**
 * A single accordion section in the sidebar. Renders as an icon-only
 * button when collapsed, or as a full header + expandable content
 * when expanded. Both states use the same vertical rhythm so icons
 * don't jump when toggling between collapsed and expanded.
 */
export const SidebarAccordion: FC<SidebarAccordionProps> = ({
	icon: Icon,
	label,
	children,
	open,
	onToggle,
}) => {
	const { collapsed } = useSidebarContext();

	if (collapsed) {
		return (
			<TooltipProvider>
				<Tooltip delayDuration={0}>
					<TooltipTrigger asChild>
						{/* Match the expanded button's px-3 py-2 and icon
						    size so icons sit at the same vertical position
						    regardless of collapsed state. */}
						<button
							type="button"
							onClick={onToggle}
							className="flex items-center px-3 py-2 rounded-md cursor-pointer bg-transparent border-none hover:bg-surface-secondary"
						>
							<Icon className="size-4 flex-shrink-0 text-content-secondary" />
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
					className="flex w-full items-center gap-2 px-3 py-2 rounded-md cursor-pointer bg-transparent border-none hover:bg-surface-secondary transition-colors"
				>
					<Icon className="size-4 flex-shrink-0 text-content-secondary" />
					<span
						className={cn(
							"text-sm font-medium text-content-secondary whitespace-nowrap",
							open && "text-content-primary",
						)}
					>
						{label}
					</span>
					<ChevronDownIcon
						open={open}
						className="size-4 text-content-secondary ml-auto flex-shrink-0"
					/>
				</button>
			</CollapsibleTrigger>
			<CollapsibleContent>
				{/* pl-6 aligns sub-item text with the section label
				    text (past the icon + gap). */}
				<div className="pl-6">{children}</div>
			</CollapsibleContent>
		</Collapsible>
	);
};
