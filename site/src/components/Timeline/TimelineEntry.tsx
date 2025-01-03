import type { Interpolation } from "@emotion/react";
import TableRow, { type TableRowProps } from "@mui/material/TableRow";
import { forwardRef } from "react";
import type { Theme } from "theme";

interface TimelineEntryProps extends TableRowProps {
	clickable?: boolean;
}

export const TimelineEntry = forwardRef<
	HTMLTableRowElement,
	TimelineEntryProps
>(function TimelineEntry({ children, clickable = true, ...props }, ref) {
	return (
		<TableRow
			ref={ref}
			css={[styles.row, clickable ? styles.clickable : null]}
			{...props}
		>
			{children}
		</TableRow>
	);
});

const styles = {
	row: (theme) => ({
		"--side-padding": "32px",
		"&:focus": {
			outlineStyle: "solid",
			outlineOffset: -1,
			outlineWidth: 2,
			outlineColor: theme.palette.primary.main,
		},
		"& td": {
			position: "relative",
			overflow: "hidden",
		},
		"& td:before": {
			"--line-width": "2px",
			position: "absolute",
			left: "calc((var(--side-padding) + var(--avatar-default)/2) - var(--line-width) / 2)",
			display: "block",
			content: "''",
			height: "100%",
			width: "var(--line-width)",
			background: theme.palette.divider,
		},
	}),

	clickable: (theme) => ({
		cursor: "pointer",

		"&:hover": {
			backgroundColor: theme.palette.action.hover,
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
