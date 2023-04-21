import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import Skeleton from "@material-ui/lab/Skeleton"
import { AvatarDataSkeleton } from "components/AvatarData/AvatarDataSkeleton"
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

export const TableLoaderSkeleton: FC<{
  columns: number
  rows?: number
  useAvatarData?: boolean
}> = ({ columns, rows = 4, useAvatarData = false }) => {
  const placeholderColumns = Array(columns).fill(undefined)
  const placeholderRows = Array(rows).fill(undefined)

  return (
    <>
      {placeholderRows.map((_, rowIndex) => (
        <TableRow key={rowIndex} role="progressbar" data-testid="loader">
          {placeholderColumns.map((_, columnIndex) => {
            if (useAvatarData && columnIndex === 0) {
              return (
                <TableCell key={columnIndex}>
                  <AvatarDataSkeleton />
                </TableCell>
              )
            }

            return (
              <TableCell key={columnIndex}>
                <Skeleton variant="text" width="25%" />
              </TableCell>
            )
          })}
        </TableRow>
      ))}
    </>
  )
}
