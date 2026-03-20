import { ChevronDownIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { useState } from "react";
import { cn } from "utils/cn";

interface ToolCollapsibleProps {
	children: ReactNode;
	header: ReactNode;
	hasContent?: boolean;
	defaultExpanded?: boolean;
	className?: string;
	headerClassName?: string;
}

export const ToolCollapsible: FC<ToolCollapsibleProps> = ({
	children,
	header,
	hasContent = true,
	defaultExpanded = false,
	className,
	headerClassName,
}) => {
	const [expanded, setExpanded] = useState(defaultExpanded);

	const headerContent = (
		<>
			{hasContent && (
				<ChevronDownIcon
					className={cn(
						"h-3.5 w-3.5 shrink-0 text-content-secondary transition-transform",
						expanded ? "rotate-0" : "-rotate-90",
					)}
				/>
			)}
			<div className="min-w-0 flex flex-1 items-center gap-2">{header}</div>
		</>
	);

	const containerClasses = hasContent
		? expanded
			? "overflow-hidden rounded-lg border border-solid border-border-default/50 bg-surface-secondary/20"
			: "rounded-lg border border-solid border-border-default/30 bg-surface-secondary/20"
		: "rounded-lg bg-surface-secondary/30";

	const headerBg = hasContent
		? expanded
			? "bg-surface-tertiary"
			: "bg-surface-tertiary/50"
		: "";

	return (
		<div className={cn(containerClasses, className)}>
			{hasContent ? (
				<button
					type="button"
					aria-expanded={expanded}
					onClick={() => setExpanded(!expanded)}
					className={cn(
						"m-0 flex w-full cursor-pointer items-center gap-2 border-0 px-3 py-1.5 text-left font-[inherit] text-[inherit] transition-colors hover:bg-surface-tertiary/60",
						headerBg,
						headerClassName,
					)}
				>
					{headerContent}
				</button>
			) : (
				<div
					className={cn(
						"flex w-full items-center gap-2 px-3 py-1.5",
						headerBg,
						headerClassName,
					)}
				>
					{headerContent}
				</div>
			)}
			{expanded && hasContent && (
				<div className="border-t border-border-default/20">{children}</div>
			)}
		</div>
	);
};
