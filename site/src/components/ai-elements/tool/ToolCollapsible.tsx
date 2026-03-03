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
	return (
		<div className={className}>
			{hasContent ? (
				<div
					role="button"
					tabIndex={0}
					aria-expanded={expanded}
					onClick={() => setExpanded(!expanded)}
					onKeyDown={(e) => {
						if (e.key === "Enter" || e.key === " ") {
							e.preventDefault();
							setExpanded(!expanded);
						}
					}}
					className={cn(
						"flex items-center gap-2 cursor-pointer",
						headerClassName,
					)}
				>
					{header}
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 shrink-0 text-content-secondary transition-transform",
							expanded ? "rotate-0" : "-rotate-90",
						)}
					/>
				</div>
			) : (
				<div className={cn("flex items-center gap-2", headerClassName)}>
					{header}
				</div>
			)}
			{expanded && hasContent && children}
		</div>
	);
};
