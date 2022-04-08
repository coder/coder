import ListItem from "@material-ui/core/ListItem"
import { makeStyles } from "@material-ui/core/styles"
import SvgIcon from "@material-ui/core/SvgIcon"
import CheckIcon from "@material-ui/icons/Check"
import React from "react"
import { NavLink } from "react-router-dom"
import { ellipsizeText } from "../../util/ellipsizeText"
import { Typography } from "../Typography/Typography"

type BorderedMenuRowVariant = "narrow" | "wide"

interface BorderedMenuRowProps {
  /** `true` indicates this row is currently selected */
  active?: boolean
  /** Optional description that appears beneath the title */
  description?: string
  /** An SvgIcon that will be rendered to the left of the title */
  Icon: typeof SvgIcon
  /** URL path */
  path?: string
  /** Required title of this row */
  title: string
  /** Defaults to `"wide"` */
  variant?: BorderedMenuRowVariant
  /** Callback fired when this row is clicked */
  onClick?: () => void
}

export const BorderedMenuRow: React.FC<BorderedMenuRowProps> = ({
  active,
  description,
  Icon,
  path,
  title,
  variant,
  onClick,
}) => {
  const styles = useStyles()

  const Component = () => (
    <ListItem
      classes={{ gutters: styles.rootGutters }}
      className={styles.root}
      onClick={onClick}
      data-status={active ? "active" : "inactive"}
    >
      <div className={styles.content} data-variant={variant}>
        <div className={styles.contentTop}>
          <Icon className={styles.icon} />
          <Typography className={styles.title}>{title}</Typography>
          {active && <CheckIcon className={styles.checkMark} />}
        </div>

        {description && (
          <Typography className={styles.description} color="textSecondary" variant="caption">
            {ellipsizeText(description)}
          </Typography>
        )}
      </div>
    </ListItem>
  )

  if (path) {
    return (
      <NavLink to={path} className={styles.link}>
        <Component />
      </NavLink>
    )
  }

  return <Component />
}

const iconSize = 20

const useStyles = makeStyles((theme) => ({
  root: {
    cursor: "pointer",
    padding: `0 ${theme.spacing(1)}px`,

    "&:hover": {
      backgroundColor: "unset",
      "& $content": {
        backgroundColor: theme.palette.background.default,
      },
    },

    "&[data-status='active']": {
      color: theme.palette.primary.main,
      "& .BorderedMenuRow-description": {
        color: theme.palette.text.primary,
      },
      "& .BorderedMenuRow-icon": {
        color: theme.palette.primary.main,
      },
    },
  },
  rootGutters: {
    padding: `0 ${theme.spacing(1.5)}px`,
  },
  content: {
    borderRadius: 7,
    display: "flex",
    flexDirection: "column",
    padding: theme.spacing(2),
    width: 320,

    "&[data-variant='narrow']": {
      width: 268,
    },
  },
  contentTop: {
    alignItems: "center",
    display: "flex",
  },
  icon: {
    color: theme.palette.text.secondary,
    height: iconSize,
    width: iconSize,

    "& path": {
      fill: theme.palette.text.secondary,
    },
  },
  link: {
    textDecoration: "none",
    color: "inherit",
  },
  title: {
    fontSize: 16,
    fontWeight: 500,
    lineHeight: 1.5,
    marginLeft: theme.spacing(2),
  },
  checkMark: {
    height: iconSize,
    marginLeft: "auto",
    width: iconSize,
  },
  description: {
    marginLeft: theme.spacing(4.5),
    marginTop: theme.spacing(0.5),
  },
}))
