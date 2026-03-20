import { ChevronDownIcon, WrenchIcon } from "lucide-react";
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
	icon = <WrenchIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />,
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
		<div className={className}>
			{hasContent ? (
				<button
					type="button"
					aria-expanded={expanded}
					onClick={() => setExpanded(!expanded)}
					className={cn(
						"m-0 flex w-full cursor-pointer items-center gap-2 border-0 bg-transparent p-0 text-left font-[inherit] text-[inherit]",
						headerClassName,
					)}
				>
					{headerContent}
				</button>
			) : (
				<div className={cn("flex w-full items-center gap-2", headerClassName)}>
					{headerContent}
				</div>
			)}
			{expanded && hasContent && children}
		</div>
	);
};
