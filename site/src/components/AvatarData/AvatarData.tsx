import Avatar from "@material-ui/core/Avatar"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import { FC, PropsWithChildren } from "react"
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
  avatar?: React.ReactNode
}

export const AvatarData: FC<PropsWithChildren<AvatarDataProps>> = ({
  title,
  subtitle,
  link,
  highlightTitle,
  avatar,
}) => {
  const styles = useStyles()

  if (!avatar) {
    avatar = <Avatar>{firstLetter(title)}</Avatar>
  }

  return (
    <div className={styles.root}>
      <div className={styles.avatarWrapper}>{avatar}</div>

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
  avatarWrapper: {
    marginRight: theme.spacing(1.5),
  },
}))
