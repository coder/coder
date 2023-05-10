import { makeStyles } from "@mui/styles"
import TableCell from "@mui/material/TableCell"
import TableRow from "@mui/material/TableRow"
import { FC } from "react"
import {
  EmptyState,
  EmptyStateProps,
} from "../../components/EmptyState/EmptyState"

export type TableEmptyProps = EmptyStateProps

export const TableEmpty: FC<TableEmptyProps> = (props) => {
  const styles = useStyles()

  return (
    <TableRow>
      <TableCell colSpan={999} className={styles.tableCell}>
        <EmptyState {...props} />
      </TableCell>
    </TableRow>
  )
}

const useStyles = makeStyles(() => ({
  tableCell: {
    padding: "0 !important",
  },
}))
