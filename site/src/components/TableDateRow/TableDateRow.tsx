import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import formatRelative from "date-fns/formatRelative"
import { FC } from "react"

export interface TableDateRow {
  date: Date
}

export const TableDateRow: FC<TableDateRow> = ({ date }) => {
  const styles = useStyles()
  // We only want the message related to the date since the time is displayed
  // inside of the build row
  const displayDate = formatRelative(date, new Date()).split("at")[0]

  return (
    <TableRow className={styles.dateRow}>
      <TableCell className={styles.dateCell} title={date.toLocaleDateString()}>
        {displayDate}
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
