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
							className="flex items-center justify-center p-2 rounded-md cursor-pointer bg-transparent border-none hover:bg-surface-secondary"
						>
							{" "}
							<Icon className="size-5 text-content-secondary" />
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
					{" "}
					<Icon className="size-4 text-content-secondary" />
					<span
						className={cn(
							"text-sm font-medium text-content-secondary",
							open && "text-content-primary",
						)}
					>
						{label}
					</span>
					<ChevronDownIcon
						open={open}
						className="size-4 text-content-secondary ml-auto"
					/>
				</button>
			</CollapsibleTrigger>
			<CollapsibleContent>
				<div className="pl-9">{children}</div>
			</CollapsibleContent>
		</Collapsible>
	);
};
