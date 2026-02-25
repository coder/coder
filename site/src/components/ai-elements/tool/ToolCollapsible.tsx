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
			<div
				role={hasContent ? "button" : undefined}
				tabIndex={hasContent ? 0 : undefined}
				aria-expanded={hasContent ? expanded : undefined}
				onClick={hasContent ? () => setExpanded(!expanded) : undefined}
				onKeyDown={
					hasContent
						? (e) => {
								if (e.key === "Enter" || e.key === " ") {
									e.preventDefault();
									setExpanded(!expanded);
								}
							}
						: undefined
				}
				className={cn(
					"flex items-center gap-2",
					hasContent && "cursor-pointer",
					headerClassName,
				)}
			>
				{header}
				{hasContent && (
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 shrink-0 text-content-secondary transition-transform",
							expanded ? "rotate-0" : "-rotate-90",
						)}
					/>
				)}
			</div>
			{expanded && hasContent && children}
		</div>
	);
};
