import TableRow from "@material-ui/core/TableRow"
import { FC, ReactNode } from "react"

export const TableHeaderRow: FC<{ children: ReactNode }> = ({ children }) => {
  return <TableRow>{children}</TableRow>
}
