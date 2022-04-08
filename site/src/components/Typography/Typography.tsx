/**
 * @fileoverview (TODO: Grey) This file is in a temporary state and is a
 * verbatim port from `@coder/ui`.
 */

import { makeStyles } from "@material-ui/core/styles"
import MuiTypography, { TypographyProps as MuiTypographyProps } from "@material-ui/core/Typography"
import * as React from "react"
import { appendCSSString, combineClasses } from "../../util/combineClasses"

export interface TypographyProps extends MuiTypographyProps {
  short?: boolean
}

/**
 * Wrapper around Material UI's Typography component to allow for future
 * custom typography types.
 *
 * See original component's Material UI documentation here: https://material-ui.com/components/typography/
 */
export const Typography: React.FC<TypographyProps> = ({ className, short, ...rest }) => {
  const styles = useStyles()

  let classes = combineClasses({ [styles.short]: short })
  if (className) {
    classes = appendCSSString(classes ?? "", className)
  }

  return <MuiTypography {...rest} className={classes} />
}

const useStyles = makeStyles({
  short: {
    "&.MuiTypography-body1": {
      lineHeight: "21px",
    },
    "&.MuiTypography-body2": {
      lineHeight: "18px",
      letterSpacing: 0.2,
    },
  },
})
