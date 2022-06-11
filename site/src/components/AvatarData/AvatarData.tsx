import Avatar from "@material-ui/core/Avatar"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import { combineClasses } from "../../util/combineClasses"
import { firstLetter } from "../../util/firstLetter"

export interface AvatarDataProps {
  title: string
  subtitle: string
  link?: string
}

export const AvatarData: FC<AvatarDataProps> = ({ title, subtitle, link }) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <Avatar variant="square" className={styles.avatar}>
        {firstLetter(title)}
      </Avatar>

      {link ? (
        <Link component={RouterLink} to={link} className={combineClasses([styles.info, styles.link])}>
          <b>{title}</b>
          <span>{subtitle}</span>
        </Link>
      ) : (
        <div className={styles.info}>
          <b>{title}</b>
          <span>{subtitle}</span>
        </div>
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
    borderRadius: 2,
    marginRight: theme.spacing(1),
    width: 24,
    height: 24,
    fontSize: 16,
  },
  info: {
    display: "flex",
    flexDirection: "column",
    color: theme.palette.text.primary,

    "& span": {
      fontSize: 12,
      color: theme.palette.text.secondary,
    },
  },
  link: {
    textDecoration: "none",
    "&:hover": {
      textDecoration: "underline",
    },
  },
}))
