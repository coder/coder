import { TableRow, type TableRowProps } from "components/Table/Table";
import { cn } from "utils/cn";

interface TimelineEntryProps extends TableRowProps {
	ref?: React.Ref<HTMLTableRowElement>;
	clickable?: boolean;
}

export const TimelineEntry: React.FC<TimelineEntryProps> = ({
	children,
	clickable = true,
	className,
	...props
}) => {
	return (
		<TableRow
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
};
