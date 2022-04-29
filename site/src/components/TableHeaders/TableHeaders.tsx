import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import React from "react"

export interface TableHeadersProps {
  columns: string[]
  hasMenu?: boolean
}

export const TableHeaders: React.FC<TableHeadersProps> = ({ columns, hasMenu }) => {
  const styles = useStyles()
  return (
    <TableRow className={styles.root}>
      {columns.map((c, idx) => (
        <TableCell key={idx} size="small">
          {c}
        </TableCell>
      ))}
      {/* 1% is a trick to make the table cell width fit the content */}
      {hasMenu && <TableCell width="1%" />}
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
