import TableRow, { type TableRowProps } from "@mui/material/TableRow";
import { type PropsWithChildren, forwardRef } from "react";

type TimelineEntryProps = PropsWithChildren<
  TableRowProps & {
    clickable?: boolean;
  }
>;

export const TimelineEntry = forwardRef(function TimelineEntry(
  { children, clickable = true, ...props }: TimelineEntryProps,
  ref?: React.ForwardedRef<HTMLTableRowElement>,
) {
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
            outlineColor: theme.palette.secondary.dark,
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
