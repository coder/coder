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
	icon?: ReactNode | null;
}

export const ToolCollapsible: FC<ToolCollapsibleProps> = ({
	children,
	header,
	hasContent = true,
	defaultExpanded = false,
	className,
	headerClassName,
	icon = null,
}) => {
	const [expanded, setExpanded] = useState(defaultExpanded);

	const headerContent = (
		<>
			{icon}
			<div className="min-w-0 flex flex-1 items-center gap-2">{header}</div>
			{hasContent && (
				<ChevronDownIcon
					className={cn(
						"h-3 w-3 shrink-0 text-content-secondary transition-transform",
						expanded ? "rotate-0" : "-rotate-90",
					)}
				/>
			)}
		</>
	);

	return (
		<div
			className={cn(
				"overflow-hidden rounded-lg border border-solid border-border-default/30 bg-surface-secondary/20",
				className,
			)}
		>
			{hasContent ? (
				<button
					type="button"
					aria-expanded={expanded}
					onClick={() => setExpanded(!expanded)}
					className={cn(
						"m-0 flex w-full cursor-pointer items-center gap-2 border-0 bg-transparent px-3 py-1.5 text-left font-[inherit] text-[inherit] transition-colors hover:bg-surface-tertiary/30",
						headerClassName,
					)}
				>
					{headerContent}
				</button>
			) : (
				<div
					className={cn(
						"flex w-full items-center gap-2 px-3 py-1.5",
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
