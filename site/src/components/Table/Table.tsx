import Box from "@material-ui/core/Box"
import MuiTable from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import React from "react"
import { TableHeaders } from "./TableHeaders"
import { TableTitle } from "./TableTitle"

export interface Column<T> {
  /**
   * The field of type T that this column is associated with
   */
  key: keyof T
  /**
   * Friendly name of the field, shown in headers
   */
  name: string
  /**
   * Custom render for the field inside the table
   */
  renderer?: (field: T[keyof T], data: T) => React.ReactElement
}

export interface TableProps<T> {
  /**
   * Title of the table
   */
  title?: string
  /**
   * A list of columns, including the name and the key
   */
  columns: Column<T>[]
  /**
   * The actual data to show in the table
   */
  data: T[]
  /**
   * Optional empty state UI when the data is empty
   */
  emptyState?: React.ReactElement
}

export const Table = <T,>({ columns, data, emptyState, title }: TableProps<T>): React.ReactElement => {
  const columnNames = columns.map(({ name }) => name)
  const body = renderTableBody(data, columns, emptyState)

  return (
    <MuiTable>
      <TableHead>
        {title && <TableTitle title={title} />}
        <TableHeaders columns={columnNames} />
      </TableHead>
      {body}
    </MuiTable>
  )
}

/**
 * Helper function to render the table data, falling back to an empty state if available
 */
const renderTableBody = <T,>(data: T[], columns: Column<T>[], emptyState?: React.ReactElement) => {
  if (data.length > 0) {
    const rows = data.map((item: T, index) => {
      const cells = columns.map((column) => {
        if (column.renderer) {
          return <TableCell key={String(column.key)}>{column.renderer(item[column.key], item)}</TableCell>
        } else {
          return <TableCell key={String(column.key)}>{String(item[column.key]).toString()}</TableCell>
        }
      })
      return <TableRow key={index}>{cells}</TableRow>
    })
    return <TableBody>{rows}</TableBody>
  } else {
    return (
      <TableBody>
        <TableRow>
          <TableCell colSpan={999}>
            <Box p={4}>{emptyState}</Box>
          </TableCell>
        </TableRow>
      </TableBody>
    )
  }
}
