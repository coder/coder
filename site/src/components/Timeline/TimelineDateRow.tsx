import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import formatRelative from "date-fns/formatRelative"
import { FC } from "react"

export interface TimelineDateRow {
  date: Date
}

// We only want the message related to the date since the time is displayed
// inside of the build row
export const createDisplayDate = (date: Date, base = new Date()): string =>
  formatRelative(date, base).split(" at ")[0]

export const TimelineDateRow: FC<TimelineDateRow> = ({ date }) => {
  const styles = useStyles()

  return (
    <TableRow className={styles.dateRow}>
      <TableCell className={styles.dateCell} title={date.toLocaleDateString()}>
        {createDisplayDate(date)}
      </TableCell>
    </TableRow>
  )
}

const useStyles = makeStyles((theme) => ({
  dateRow: {
    background: theme.palette.background.paper,

    "&:not(:first-child) td": {
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
}))
