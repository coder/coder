import { makeStyles, Theme } from "@material-ui/core/styles"
import { FC } from "react"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"
import { CopyButton } from "../CopyButton/CopyButton"

export interface CodeExampleProps {
  code: string
  className?: string
  buttonClassName?: string
  tooltipTitle?: string
  inline?: boolean
}

/**
 * Component to show single-line code examples, with a copy button
 */
export const CodeExample: FC<React.PropsWithChildren<CodeExampleProps>> = ({
  code,
  className,
  buttonClassName,
  tooltipTitle,
  inline,
}) => {
  const styles = useStyles({ inline: inline })

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

interface styleProps {
  inline?: boolean
}

const useStyles = makeStyles<Theme, styleProps>((theme) => ({
  root: (props) => ({
    display: props.inline ? "inline-flex" : "flex",
    flexDirection: "row",
    alignItems: "center",
    background: props.inline ? "rgb(0 0 0 / 30%)" : "hsl(223, 27%, 3%)",
    border: props.inline ? undefined : `1px solid ${theme.palette.divider}`,
    color: theme.palette.primary.contrastText,
    fontFamily: MONOSPACE_FONT_FAMILY,
    fontSize: 14,
    borderRadius: theme.shape.borderRadius,
    padding: props.inline ? "0px" : theme.spacing(0.5),
  }),
  code: (props) => ({
    padding: `
      ${props.inline ? 0 : theme.spacing(0.5)}px
      ${theme.spacing(0.75)}px
      ${props.inline ? 0 : theme.spacing(0.5)}px
      ${props.inline ? theme.spacing(1) : theme.spacing(2)}px
    `,
    width: "100%",
    display: "flex",
    alignItems: "center",
    wordBreak: "break-all",
  }),
  button: (props) => ({
    border: 0,
    minWidth: props.inline ? 30 : 42,
    minHeight: props.inline ? 30 : 42,
    borderRadius: theme.shape.borderRadius,
    padding: props.inline ? theme.spacing(0.4) : undefined,
    background: "transparent",
  }),
}))
