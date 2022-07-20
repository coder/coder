import Avatar from "@material-ui/core/Avatar"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import { firstLetter } from "../../util/firstLetter"
import {
  TableCellData,
  TableCellDataPrimary,
  TableCellDataSecondary,
} from "../TableCellData/TableCellData"

export interface AvatarDataProps {
  title: string
  subtitle: string
  highlightTitle?: boolean
  link?: string
}

export const AvatarData: FC<AvatarDataProps> = ({ title, subtitle, link, highlightTitle }) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <Avatar className={styles.avatar}>{firstLetter(title)}</Avatar>

      {link ? (
        <Link to={link} underline="none" component={RouterLink}>
          <TableCellData>
            <TableCellDataPrimary highlight={highlightTitle}>{title}</TableCellDataPrimary>
            <TableCellDataSecondary>{subtitle}</TableCellDataSecondary>
          </TableCellData>
        </Link>
      ) : (
        <TableCellData>
          <TableCellDataPrimary highlight={highlightTitle}>{title}</TableCellDataPrimary>
          <TableCellDataSecondary>{subtitle}</TableCellDataSecondary>
        </TableCellData>
      )}
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    display: "flex",
    alignItems: "center",
  },
  avatar: {
    marginRight: theme.spacing(1.5),
  },
}))
