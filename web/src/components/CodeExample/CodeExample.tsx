import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"
import { CopyButton } from "../CopyButton/CopyButton"

export interface CodeExampleProps {
  code: string
  className?: string
  buttonClassName?: string
  tooltipTitle?: string
}

/**
 * Component to show single-line code examples, with a copy button
 */
export const CodeExample: FC<React.PropsWithChildren<CodeExampleProps>> = ({
  code,
  className,
  buttonClassName,
  tooltipTitle,
}) => {
  const styles = useStyles()

  return (
    <div className={combineClasses([styles.root, className])}>
      <code className={styles.code}>{code}</code>
      <CopyButton
        text={code}
        tooltipTitle={tooltipTitle}
        buttonClassName={combineClasses([styles.button, buttonClassName])}
      />
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
    width: "100%",
    display: "flex",
    alignItems: "center",
    wordBreak: "break-all",
  },
  button: {
    border: 0,
    minWidth: 42,
    minHeight: 42,
    borderRadius: theme.shape.borderRadius,
  },
}))
