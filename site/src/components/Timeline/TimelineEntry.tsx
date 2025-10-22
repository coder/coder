import { TableRow, type TableRowProps } from "components/Table/Table";
import { forwardRef } from "react";
import { cn } from "utils/cn";

interface TimelineEntryProps extends TableRowProps {
	clickable?: boolean;
}

export const TimelineEntry = forwardRef<
	HTMLTableRowElement,
	TimelineEntryProps
>(({ children, clickable = true, className, ...props }, ref) => {
	return (
		<TableRow
			ref={ref}
			className={cn(
				"focus:outline focus:-outline-offset-1 focus:outline-2 focus:outline-content-primary ",
				"[&_td]:relative [&_td]:overflow-hidden",
				"[&_td:before]:absolute [&_td:before]:block [&_td:before]:h-full [&_td:before]:content-[''] [&_td:before]:bg-border [&_td:before]:w-0.5 [&_td:before]:left-[calc((32px+(var(--avatar-default)/2))-1px)]",
				clickable && "cursor-pointer hover:bg-surface-secondary",
				className,
			)}
			{...props}
		>
			{children}
		</TableRow>
	);
});
