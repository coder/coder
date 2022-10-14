import { makeStyles } from "@material-ui/core/styles"
import { Stack } from "components/Stack/Stack"
import React, { PropsWithChildren } from "react"
import { combineClasses } from "util/combineClasses"

export const EnabledBadge: React.FC = () => {
  const styles = useStyles()
  return (
    <span className={combineClasses([styles.badge, styles.enabledBadge])}>
      Enabled
    </span>
  )
}

export const DisabledBadge: React.FC = () => {
  const styles = useStyles()
  return (
    <span className={combineClasses([styles.badge, styles.disabledBadge])}>
      Disabled
    </span>
  )
}

export const EnterpriseBadge: React.FC = () => {
  const styles = useStyles()
  return (
    <span className={combineClasses([styles.badge, styles.enterpriseBadge])}>
      Enterprise
    </span>
  )
}

export const Badges: React.FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles()
  return (
    <Stack
      className={styles.badges}
      direction="row"
      alignItems="center"
      spacing={1}
    >
      {children}
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  badges: {
    margin: theme.spacing(0, 0, 2),
  },

  badge: {
    fontSize: 10,
    height: 24,
    fontWeight: 600,
    textTransform: "uppercase",
    letterSpacing: "0.085em",
    padding: theme.spacing(0, 1.5),
    borderRadius: 9999,
    display: "flex",
    alignItems: "center",
    width: "fit-content",
  },

  enterpriseBadge: {
    backgroundColor: theme.palette.info.dark,
    border: `1px solid ${theme.palette.info.light}`,
  },

  enabledBadge: {
    border: `1px solid ${theme.palette.success.light}`,
    backgroundColor: theme.palette.success.dark,
  },

  disabledBadge: {
    border: `1px solid ${theme.palette.divider}`,
    backgroundColor: theme.palette.background.paper,
  },
}))
