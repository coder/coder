import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"
import { CopyButton } from "../CopyButton/CopyButton"

export interface CodeExampleProps {
  code: string
  className?: string
  buttonClassName?: string
}

/**
 * Component to show single-line code examples, with a copy button
 */
export const CodeExample: FC<React.PropsWithChildren<CodeExampleProps>> = ({
  code,
  className,
  buttonClassName,
}) => {
  const styles = useStyles()

  return (
    <div className={combineClasses([styles.root, className])}>
      <code className={styles.code}>{code}</code>
      <CopyButton text={code} buttonClassName={combineClasses([styles.button, buttonClassName])} />
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    display: "flex",
    flexDirection: "row",
    alignItems: "center",
    background: "hsl(223, 27%, 3%)",
    border: `1px solid ${theme.palette.divider}`,
    color: theme.palette.primary.contrastText,
    fontFamily: MONOSPACE_FONT_FAMILY,
    fontSize: 14,
    borderRadius: theme.shape.borderRadius,
    padding: theme.spacing(0.5),
  },
  code: {
    padding: `
      ${theme.spacing(0.5)}px
      ${theme.spacing(0.75)}px
      ${theme.spacing(0.5)}px
      ${theme.spacing(2)}px
    `,
    whiteSpace: "nowrap",
    width: "100%",
    overflowX: "auto",
    // Have a better area to display the scrollbar
    height: 42,
    display: "flex",
    alignItems: "center",
  },
  button: {
    border: 0,
    minWidth: 42,
    minHeight: 42,
    borderRadius: theme.shape.borderRadius,
  },
}))
