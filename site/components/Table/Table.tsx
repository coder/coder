import React from "react"
import Box from "@material-ui/core/Box"
import { makeStyles } from "@material-ui/core/styles"
import Paper from "@material-ui/core/Paper"
import MuiTable from "@material-ui/core/Table"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import TableCell from "@material-ui/core/TableCell"

import { TableTitle } from "./TableTitle"
import { TableHeaders } from "./TableHeaders"

export interface Column<T> {
  key: keyof T
  name: string
  renderer?: (field: T[keyof T], data: T) => React.ReactElement
}

export interface TableProps<T> {
  title?: string
  columns: Column<T>[]
  data: T[]
  emptyState?: React.ReactElement
}

export const Table = <T,>({ columns, data, emptyState }: TableProps<T>): React.ReactElement => {
  const columnNames = columns.map(({ name }) => name)

  let body: JSX.Element
  if (data.length > 0) {
    const rows = data.map((item: T) => {
      const cells = columns.map((column) => {

        if (column.renderer) {
          return <TableCell>{column.renderer(item[column.key], item)}</TableCell>
        } else {
          return <TableCell>{item[column.key].toString()}</TableCell>
        }

        return <TableCell>{item[column.key].toString()}</TableCell>
      })
      return <TableRow>{cells}</TableRow>
    })
    body = <>{rows}</>
  } else {
    body = (
      <TableRow>
        <TableCell colSpan={999}>
          <Box p={4}>{emptyState}</Box>
        </TableCell>
      </TableRow>
    )
  }

  return (
    <MuiTable>
      <TableHead>
        <TableTitle title={"All Projects"} />
        <TableHeaders columns={columnNames} />
      </TableHead>
      {body}
    </MuiTable>
  )
}
