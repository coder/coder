import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { CopyButton } from "../CopyButton/CopyButton"

export interface CodeExampleProps {
  code: string
}

/**
 * Component to show single-line code examples, with a copy button
 */
export const CodeExample: React.FC<CodeExampleProps> = ({ code }) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <code>{code}</code>
      <CopyButton text={code} />
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    display: "flex",
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    background: theme.palette.background.default,
    color: theme.palette.primary.contrastText,
    fontFamily: MONOSPACE_FONT_FAMILY,
    fontSize: 13,
    padding: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
  },
}))
