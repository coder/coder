import { makeStyles } from "@material-ui/core/styles"
import React, { PropsWithChildren } from "react"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"

export const OptionName: React.FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles()
  return <span className={styles.optionName}>{children}</span>
}

export const OptionDescription: React.FC<PropsWithChildren> = ({
  children,
}) => {
  const styles = useStyles()
  return <span className={styles.optionDescription}>{children}</span>
}

export const OptionValue: React.FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles()
  return <span className={styles.optionValue}>{children}</span>
}

const useStyles = makeStyles((theme) => ({
  optionName: {
    display: "block",
  },
  optionDescription: {
    display: "block",
    color: theme.palette.text.secondary,
    fontSize: 14,
    marginTop: theme.spacing(0.5),
  },
  optionValue: {
    fontSize: 14,
    fontFamily: MONOSPACE_FONT_FAMILY,

    "& ul": {
      padding: theme.spacing(2),
    },
  },
}))
