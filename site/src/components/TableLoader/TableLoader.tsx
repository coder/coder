import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import { FC } from "react"
import { Loader } from "../Loader/Loader"

export const TableLoader: FC = () => {
  const styles = useStyles()

  return (
    <TableRow>
      <TableCell colSpan={999} className={styles.cell}>
        <Loader />
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
