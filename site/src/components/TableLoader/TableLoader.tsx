import CircularProgress from "@material-ui/core/CircularProgress"
import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import React from "react"

export const TableLoader: React.FC = () => {
  const styles = useStyles()

  return (
    <TableRow>
      <TableCell colSpan={999} className={styles.cell}>
        <CircularProgress size={30} />
      </TableCell>
    </TableRow>
  )
}

const useStyles = makeStyles((theme) => ({
  cell: {
    textAlign: "center",
    height: theme.spacing(20),
  },
}))
