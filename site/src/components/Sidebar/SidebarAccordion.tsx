import type { ElementType, FC, ReactNode } from "react";
import { Link } from "react-router";
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
	/** URL to navigate to when clicking the icon in collapsed mode. */
	href?: string;
	/** Whether this section contains the current route. */
	active?: boolean;
}

export const SidebarAccordion: FC<SidebarAccordionProps> = ({
	icon: Icon,
	label,
	children,
	open,
	onToggle,
	href,
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

	if (collapsed) {
		return (
			<TooltipProvider>
				<Tooltip delayDuration={0}>
					<TooltipTrigger asChild>
						{href ? (
							<Link
								to={href}
								onClick={expand}
								className="flex items-center justify-center w-10 h-10 rounded-md no-underline hover:bg-surface-secondary"
							>
								<Icon className={iconClass} />
							</Link>
						) : (
							<button
								type="button"
								onClick={() => {
									expand();
									onToggle();
								}}
								className="flex items-center justify-center w-10 h-10 rounded-md cursor-pointer bg-transparent border-none hover:bg-surface-secondary"
							>
								<Icon className={iconClass} />
							</button>
						)}
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
					<Icon className={iconClass} />
					<span className={labelClass}>{label}</span>
					<ChevronDownIcon
						open={open}
						className="size-4 text-content-secondary ml-auto flex-shrink-0"
					/>
				</button>
			</CollapsibleTrigger>
			<CollapsibleContent>
				<div className="pl-6">{children}</div>
			</CollapsibleContent>
		</Collapsible>
	);
};
