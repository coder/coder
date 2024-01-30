import TableRow, { type TableRowProps } from "@mui/material/TableRow";
import { forwardRef } from "react";

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
      css={(theme) => [
        {
          position: "relative",
          "&:focus": {
            outlineStyle: "solid",
            outlineOffset: -1,
            outlineWidth: 2,
            outlineColor: theme.palette.primary.main,
          },
          "& td:before": {
            position: "absolute",
            left: 50,
            display: "block",
            content: "''",
            height: "100%",
            width: 2,
            background: theme.palette.divider,
          },
        },
        clickable && {
          cursor: "pointer",

          "&:hover": {
            backgroundColor: theme.palette.action.hover,
          },
        },
      ]}
      {...props}
    >
      {children}
    </TableRow>
  );
});
