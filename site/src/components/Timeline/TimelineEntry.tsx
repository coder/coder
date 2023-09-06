import { makeStyles } from "@mui/styles";
import TableRow, { TableRowProps } from "@mui/material/TableRow";
import { PropsWithChildren } from "react";
import { combineClasses } from "utils/combineClasses";

interface TimelineEntryProps {
  clickable?: boolean;
}

export const TimelineEntry = ({
  children,
  clickable = true,
  ...props
}: PropsWithChildren<TimelineEntryProps & TableRowProps>): JSX.Element => {
  const styles = useStyles();
  return (
    <TableRow
      className={combineClasses({
        [styles.timelineEntry]: true,
        [styles.clickable]: clickable,
      })}
      {...props}
    >
      {children}
    </TableRow>
  );
};

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
