import Link from "@material-ui/core/Link"
import Popover from "@material-ui/core/Popover"
import { makeStyles } from "@material-ui/core/styles"
import HelpIcon from "@material-ui/icons/HelpOutline"
import OpenInNewIcon from "@material-ui/icons/OpenInNew"
import { useState } from "react"
import { Stack } from "../Stack/Stack"

export interface HelpTooltipProps {
  // Useful to test on storybook
  open?: boolean
}

export const HelpTooltip: React.FC<HelpTooltipProps> = ({ children, open }) => {
  const styles = useStyles()
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null)
  open = open ?? Boolean(anchorEl)
  const id = open ? "help-popover" : undefined

  return (
    <>
      <button aria-describedby={id} className={styles.button} onClick={(event) => setAnchorEl(event.currentTarget)}>
        <HelpIcon className={styles.icon} />
      </button>
      <Popover
        classes={{ paper: styles.popoverPaper }}
        id={id}
        open={open}
        anchorEl={anchorEl}
        onClose={() => {
          setAnchorEl(null)
        }}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "left",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "left",
        }}
      >
        {children}
      </Popover>
    </>
  )
}

export const HelpTooltipTitle: React.FC = ({ children }) => {
  const styles = useStyles()

  return <h4 className={styles.title}>{children}</h4>
}

export const HelpTooltipText: React.FC = ({ children }) => {
  const styles = useStyles()

  return <p className={styles.text}>{children}</p>
}

export const HelpTooltipLink: React.FC<{ href: string }> = ({ children, href }) => {
  const styles = useStyles()

  return (
    <Link href={href} target="_blank" rel="noreferrer" className={styles.link}>
      <OpenInNewIcon className={styles.linkIcon} />
      {children}
    </Link>
  )
}

export const HelpTooltipLinksGroup: React.FC = ({ children }) => {
  const styles = useStyles()

  return (
    <Stack spacing={1} className={styles.linksGroup}>
      {children}
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  button: {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    width: theme.spacing(3),
    height: theme.spacing(3),
    padding: 0,
    border: 0,
    background: "transparent",
    color: theme.palette.text.secondary,
    cursor: "pointer",
    marginLeft: theme.spacing(1),

    "&:hover": {
      color: theme.palette.text.primary,
    },
  },

  icon: {
    width: theme.spacing(2),
    height: theme.spacing(2),
  },

  popoverPaper: {
    marginTop: theme.spacing(0.5),
    width: theme.spacing(38),
    padding: theme.spacing(2.5),
    color: theme.palette.text.secondary,
  },

  title: {
    marginTop: 0,
    marginBottom: theme.spacing(1),
    color: theme.palette.text.primary,
  },

  text: {
    marginTop: theme.spacing(0.5),
    marginBottom: theme.spacing(0.5),
  },

  link: {
    display: "flex",
    alignItems: "center",
  },

  linkIcon: {
    color: "inherit",
    width: 14,
    height: 14,
    marginRight: theme.spacing(1),
  },

  linksGroup: {
    marginTop: theme.spacing(2),
  },
}))
