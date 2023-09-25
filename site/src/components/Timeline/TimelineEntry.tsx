import { makeStyles } from "@mui/styles";
import TableRow, { TableRowProps } from "@mui/material/TableRow";
import { type PropsWithChildren, forwardRef } from "react";
import { combineClasses } from "utils/combineClasses";

type TimelineEntryProps = PropsWithChildren<
  TableRowProps & {
    clickable?: boolean;
  }
>;

export const TimelineEntry = forwardRef(function TimelineEntry(
  { children, clickable = true, ...props }: TimelineEntryProps,
  ref?: React.ForwardedRef<HTMLTableRowElement>,
) {
  const styles = useStyles();

  return (
    <TableRow
      ref={ref}
      className={combineClasses({
        [styles.timelineEntry]: true,
        [styles.clickable]: clickable,
      })}
      {...props}
    >
      {children}
    </TableRow>
  );
});

const useStyles = makeStyles((theme) => ({
  clickable: {
    cursor: "pointer",

    "&:hover": {
      backgroundColor: theme.palette.action.hover,
    },
  },

  timelineEntry: {
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
}));
