import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"

export interface CodeBlockProps {
  lines: string[]
  ctas?: React.ReactElement[]
  className?: string
}

export const CodeBlock: React.FC<CodeBlockProps> = ({ lines, ctas, className = "" }) => {
  const styles = useStyles()

  return (
    <>
      <div className={combineClasses([styles.root, className])}>
        {lines.map((line, idx) => (
          <div className={styles.line} key={idx}>
            {line}
          </div>
        ))}
      </div>
      {ctas && ctas.length && (
        <div className={styles.ctaBar}>
          {ctas.map((cta, i) => {
            return <React.Fragment key={i}>{cta}</React.Fragment>
          })}
        </div>
      )}
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    minHeight: 156,
    maxHeight: 240,
    overflowY: "scroll",
    background: theme.palette.background.default,
    color: theme.palette.text.primary,
    fontFamily: MONOSPACE_FONT_FAMILY,
    fontSize: 13,
    wordBreak: "break-all",
    padding: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
  },
  line: {
    whiteSpace: "pre-wrap",
  },
  ctaBar: {
    display: "flex",
    justifyContent: "space-between",
  },
}))
