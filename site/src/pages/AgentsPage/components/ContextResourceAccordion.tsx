import { cn } from "cn";
import { ChevronDownIcon } from "lucide-react";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";

/** An independently collapsible section of the chat's context resources. */
export const ContextResourceAccordion: React.FC<
	React.ComponentProps<typeof Collapsible>
> = ({ className, ...props }) => (
	<Collapsible className={cn("flex flex-col", className)} {...props} />
);

type ContextResourceAccordionTriggerProps = React.ComponentProps<
	typeof CollapsibleTrigger
> & { detail?: React.ReactNode };

/** Labels a resource section with optional detail shown while collapsed. */
export const ContextResourceAccordionTrigger: React.FC<
	ContextResourceAccordionTriggerProps
> = ({ detail, children, className, ...props }) => (
	<CollapsibleTrigger
		className={cn(
			"group flex items-center justify-between gap-1 rounded h-8 pl-1 pr-2",
			"text-left text-xs font-medium text-content-secondary",
			"focus-visible:outline-hidden focus-visible:ring-2",
			"focus-visible:ring-content-link",
			"data-[state=open]:text-content-primary",
			"hover:text-content-primary",
			"transition-colors duration-200",
			className,
		)}
		{...props}
	>
		<div className="flex items-center gap-1">
			<ChevronDownIcon className="size-4 shrink-0 transition-transform group-data-[state=closed]:-rotate-90" />
			<span className="font-semibold flex items-center gap-0.5">{children}</span>
		</div>
		{detail != null && (
			<span className="font-mono font-normal text-content-secondary group-data-[state=open]:opacity-0 transition-opacity duration-25">
				{detail}
			</span>
		)}
	</CollapsibleTrigger>
);

/** Unmounts resource rows while collapsed unless forceMount is enabled. */
export const ContextResourceAccordionContent: React.FC<
	React.ComponentProps<typeof CollapsibleContent>
> = ({ className, ...props }) => (
	<CollapsibleContent
		className={cn("flex flex-col gap-2.5 pl-1 pr-2 pb-2", className)}
		{...props}
	/>
);
