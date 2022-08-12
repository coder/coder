import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import { FC } from "react"

export interface TableHeadersProps {
  columns: string[]
  hasMenu?: boolean
}

export const TableHeaderRow: FC = ({ children }) => {
  return <TableRow>{children}</TableRow>
}

export const TableHeaders: FC<TableHeadersProps> = ({ columns, hasMenu }) => {
  return (
    <TableHeaderRow>
      {columns.map((c, idx) => (
        <TableCell key={idx} size="small">
          {c}
        </TableCell>
      ))}
      {/* 1% is a trick to make the table cell width fit the content */}
      {hasMenu && <TableCell width="1%" />}
    </TableHeaderRow>
  )
}
