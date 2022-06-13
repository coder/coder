import Link from "@material-ui/core/Link"
import Popover from "@material-ui/core/Popover"
import { makeStyles } from "@material-ui/core/styles"
import HelpIcon from "@material-ui/icons/HelpOutline"
import OpenInNewIcon from "@material-ui/icons/OpenInNew"
import React, { createContext, useContext, useState } from "react"
import { Stack } from "../Stack/Stack"

type Icon = typeof HelpIcon

type Size = "small" | "medium"
export interface HelpTooltipProps {
  // Useful to test on storybook
  open?: boolean
  size?: Size
}

const HelpTooltipContext = createContext<{ open: boolean; onClose: () => void } | undefined>(undefined)

const useHelpTooltip = () => {
  const helpTooltipContext = useContext(HelpTooltipContext)

  if (!helpTooltipContext) {
    throw new Error("This hook should be used in side of the HelpTooltipContext.")
  }

  return helpTooltipContext
}

export const HelpTooltip: React.FC<HelpTooltipProps> = ({ children, open, size = "medium" }) => {
  const styles = useStyles({ size })
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null)
  open = open ?? Boolean(anchorEl)
  const id = open ? "help-popover" : undefined

  const onClose = () => {
    setAnchorEl(null)
  }

  return (
    <>
      <button
        aria-describedby={id}
        className={styles.button}
        onClick={(event) => {
          event.stopPropagation()
          setAnchorEl(event.currentTarget)
        }}
      >
        <HelpIcon className={styles.icon} />
      </button>
      <Popover
        classes={{ paper: styles.popoverPaper }}
        id={id}
        open={open}
        anchorEl={anchorEl}
        onClose={onClose}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "left",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "left",
        }}
      >
        <HelpTooltipContext.Provider value={{ open, onClose }}>{children}</HelpTooltipContext.Provider>
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

export const HelpTooltipAction: React.FC<{ icon: Icon; onClick: () => void }> = ({ children, icon: Icon, onClick }) => {
  const styles = useStyles()
  const tooltip = useHelpTooltip()

  return (
    <button
      className={styles.action}
      onClick={(event) => {
        event.stopPropagation()
        onClick()
        tooltip.onClose()
      }}
    >
      <Icon className={styles.actionIcon} />
      {children}
    </button>
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

const getButtonSpacingFromSize = (size?: Size): number => {
  switch (size) {
    case "small":
      return 2.75
    case "medium":
    default:
      return 3
  }
}

const getIconSpacingFromSize = (size?: Size): number => {
  switch (size) {
    case "small":
      return 1.75
    case "medium":
    default:
      return 2
  }
}

const useStyles = makeStyles((theme) => ({
  button: {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    width: ({ size }: { size?: Size }) => theme.spacing(getButtonSpacingFromSize(size)),
    height: ({ size }: { size?: Size }) => theme.spacing(getButtonSpacingFromSize(size)),
    padding: 0,
    border: 0,
    background: "transparent",
    color: theme.palette.text.primary,
    opacity: 0.5,
    cursor: "pointer",

    "&:hover": {
      opacity: 0.75,
    },
  },

  icon: {
    width: ({ size }: { size?: Size }) => theme.spacing(getIconSpacingFromSize(size)),
    height: ({ size }: { size?: Size }) => theme.spacing(getIconSpacingFromSize(size)),
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

  action: {
    display: "flex",
    alignItems: "center",
    background: "none",
    border: 0,
    color: theme.palette.primary.light,
    padding: 0,
    cursor: "pointer",
    fontSize: 14,
  },

  actionIcon: {
    color: "inherit",
    width: 14,
    height: 14,
    marginRight: theme.spacing(1),
  },
}))
