import Box from "@material-ui/core/Box"
import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import Typography from "@material-ui/core/Typography"
import * as React from "react"

export interface TableTitleProps {
  /** A title to display */
  readonly title?: React.ReactNode
  /** Arbitrary node to display to the right of the title. */
  readonly details?: React.ReactNode
}

/**
 * Component that encapsulates all of the pieces that sit on the top of a table.
 */
export const TableTitle: React.FC<TableTitleProps> = ({ title, details }) => {
  const styles = useStyles()
  return (
    <TableRow>
      <TableCell colSpan={9999} className={styles.cell}>
        <Box className={`${styles.container} ${details ? "-details" : ""}`}>
          {title && (
            <Typography variant="h6" className={styles.title}>
              {title}
            </Typography>
          )}
          {details && <div className={styles.details}>{details}</div>}
        </Box>
      </TableCell>
    </TableRow>
  )
}

const useStyles = makeStyles((theme) => ({
  cell: {
    background: "none",
    paddingTop: theme.spacing(2),
    paddingBottom: theme.spacing(2),
  },
  container: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
  },
  title: {
    fontSize: theme.typography.h5.fontSize,
    fontWeight: 500,
    color: theme.palette.text.primary,
    textTransform: "none",
    letterSpacing: "normal",
  },
  details: {
    alignItems: "center",
    display: "flex",
    justifyContent: "flex-end",
    letterSpacing: "normal",
    margin: `0 ${theme.spacing(2)}px`,

    [theme.breakpoints.down("sm")]: {
      margin: `${theme.spacing(1)}px 0 0 0`,
    },
  },
}))
