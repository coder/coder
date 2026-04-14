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
}

/**
 * A single accordion section in the sidebar. Both collapsed and
 * expanded states use identical px-3 py-2 padding so icons stay
 * at the same vertical position regardless of state.
 *
 * In collapsed mode, clicking the icon navigates to the section's
 * first page (via href) and expands the sidebar.
 */
export const SidebarAccordion: FC<SidebarAccordionProps> = ({
	icon: Icon,
	label,
	children,
	open,
	onToggle,
	href,
}) => {
	const { collapsed, expand } = useSidebarContext();

	if (collapsed) {
		return (
			<TooltipProvider>
				<Tooltip delayDuration={0}>
					<TooltipTrigger asChild>
						{href ? (
							<Link
								to={href}
								onClick={expand}
								className="flex w-full items-center px-3 py-2 h-10 rounded-md no-underline hover:bg-surface-secondary"
							>
								<Icon className="size-4 flex-shrink-0 text-content-secondary" />
							</Link>
						) : (
							<button
								type="button"
								onClick={() => {
									expand();
									onToggle();
								}}
								className="flex w-full items-center px-3 py-2 h-10 rounded-md cursor-pointer bg-transparent border-none hover:bg-surface-secondary"
							>
								<Icon className="size-4 flex-shrink-0 text-content-secondary" />
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
				{/* Indent sub-items past the icon (16px) + gap (8px)
				    so their text left-aligns with section label text. */}
				<div className="pl-6">{children}</div>
			</CollapsibleContent>
		</Collapsible>
	);
};
