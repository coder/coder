import { makeStyles } from "@mui/styles";
import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import { FC } from "react";
import { createDisplayDate } from "./utils";

export interface TimelineDateRow {
  date: Date;
}

export const TimelineDateRow: FC<TimelineDateRow> = ({ date }) => {
  const styles = useStyles();

  return (
    <TableRow className={styles.dateRow}>
      <TableCell className={styles.dateCell} title={date.toLocaleDateString()}>
        {createDisplayDate(date)}
      </TableCell>
    </TableRow>
  );
};

const useStyles = makeStyles((theme) => ({
  dateRow: {
    background: theme.palette.background.paper,

    "&:not(:first-of-type) td": {
      borderTop: `1px solid ${theme.palette.divider}`,
    },
  },

  dateCell: {
    padding: `${theme.spacing(1, 4)} !important`,
    background: `${theme.palette.background.paperLight} !important`,
    fontSize: 12,
    position: "relative",
    color: theme.palette.text.secondary,
    textTransform: "capitalize",
  },
}));
