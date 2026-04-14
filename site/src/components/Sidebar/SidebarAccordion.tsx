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
 * A single accordion section in the sidebar. Both collapsed and
 * expanded states use identical left padding (0) so icons always
 * sit at the nav's pl-6 boundary, aligned with the Coder logo.
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
						<button
							type="button"
							onClick={onToggle}
							className="flex items-center py-2 rounded-md cursor-pointer bg-transparent border-none hover:bg-surface-secondary"
						>
							<Icon className="size-4 flex-shrink-0 text-content-secondary -ml-px" />
						</button>{" "}
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
					className="flex w-full items-center gap-2 py-2 pr-3 rounded-md cursor-pointer bg-transparent border-none hover:bg-surface-secondary transition-colors"
				>
					<Icon className="size-4 flex-shrink-0 text-content-secondary -ml-px" />
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
