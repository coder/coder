import TableRow, { type TableRowProps } from "@mui/material/TableRow";
import { forwardRef } from "react";
import { cn } from "utils/cn";

interface TimelineEntryProps extends Omit<TableRowProps, "style"> {
	clickable?: boolean;
}

export const TimelineEntry = forwardRef<
	HTMLTableRowElement,
	TimelineEntryProps
>(({ children, clickable = true, className, ...props }, ref) => {
	return (
		<TableRow
			ref={ref}
			{...props}
			style={{ "--side-padding": "32px", "--line-width": "2px" }}
			className={cn(
				"focus:-outline-offset-1 focus:outline-2 focus:outline-border focus:outline-solid",
				"[&_td]:relative [&_td]:overflow-hidden",
				"[&_td:before]:absolute [&_td:before]:block [&_td:before]:h-full [&_td:before]:bg-border [&_td:before]:content-[''] [&_td:before]:w-[--line-width] [&_td:before]:left-[calc((var(--side-padding)+var(--avatar-default)/2)-var(--line-width)/2)]",
				clickable && "cursor-pointer hover:bg-surface-secondary",
				className,
			)}
		>
			{children}
		</TableRow>
	);
});
