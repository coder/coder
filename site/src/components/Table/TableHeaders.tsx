import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import React from "react"

export interface TableHeadersProps {
  columns: string[]
}

export const TableHeaders: React.FC<TableHeadersProps> = ({ columns }) => {
  const styles = useStyles()
  return (
    <TableRow className={styles.root}>
      {columns.map((c, idx) => (
        <TableCell key={idx} size="small">
          {c}
        </TableCell>
      ))}
    </TableRow>
  )
}

export const useStyles = makeStyles((theme) => ({
  root: {
    fontSize: 12,
    fontWeight: 500,
    lineHeight: "16px",
    letterSpacing: 1.5,
    textTransform: "uppercase",
    paddingTop: theme.spacing(1),
    paddingBottom: theme.spacing(1),
    color: theme.palette.text.secondary,
    backgroundColor: theme.palette.background.default,
  },
}))
