import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import TableCell, { TableCellProps } from "@material-ui/core/TableCell"
import { Link as RouterLink } from "react-router-dom"
import { combineClasses } from "../../util/combineClasses"

// TableCellLink wraps a TableCell filling the entirety with a Link.
// This allows table rows to be clickable with browser-behavior like ctrl+click.
export const TableCellLink: React.FC<React.PropsWithChildren<TableCellProps & {
  to: string
}>> = (props) => {
  const styles = useStyles()

  return (
    <TableCell className={styles.cell} {...props}>
      <Link
        component={RouterLink}
        to={props.to}
        className={combineClasses([styles.link, "MuiTableCell-root", "MuiTableCell-body"])}
      >
        {props.children}
      </Link>
    </TableCell>
  )
}

const useStyles = makeStyles((theme) => ({
  cell: {
    // This must override all padding for all rules on a TableCell.
    // Otherwise, the link will not cover the entire region.
    // It's unfortunate to use `!important`, but this seems to be
    // a reasonable use-case.
    padding: "0 !important",
  },
  link: {
    display: "block",
    width: "100%",
    border: "none",
    background: "none",
    paddingTop: theme.spacing(2),
    paddingBottom: theme.spacing(2),
    // This is required to hide all underlines for child elements!
    textDecoration: "none !important",
  },
}))
