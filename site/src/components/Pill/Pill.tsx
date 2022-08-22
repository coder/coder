import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"
import { PaletteIndex } from "theme/palettes"
import { combineClasses } from "util/combineClasses"

export interface PillProps {
  className?: string
  icon?: React.ReactNode
  text: string
  type?: PaletteIndex
  lightBorder?: boolean
}

export const Pill: React.FC<PillProps> = ({ className, icon, text, type, lightBorder = false }) => {
  const styles = useStyles({ icon, type, lightBorder })
  return (
    <div className={combineClasses([styles.wrapper, styles.pillColor, className])} role="status">
      {icon && <div className={styles.iconWrapper}>{icon}</div>}
      {text}
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  wrapper: {
    fontFamily: MONOSPACE_FONT_FAMILY,
    display: "inline-flex",
    alignItems: "center",
    borderWidth: 1,
    borderStyle: "solid",
    borderRadius: 99999,
    fontSize: 14,
    fontWeight: 500,
    color: "#FFF",
    height: theme.spacing(3),
    paddingLeft: ({ icon }: { icon?: React.ReactNode }) =>
      icon ? theme.spacing(0.75) : theme.spacing(1.5),
    paddingRight: theme.spacing(1.5),
    whiteSpace: "nowrap",
  },

  pillColor: {
    backgroundColor: ({ type }: { type?: PaletteIndex }) =>
      type ? theme.palette[type].dark : theme.palette.text.secondary,
    borderColor: ({ type, lightBorder }: { type?: PaletteIndex; lightBorder?: boolean }) =>
      type
        ? lightBorder
          ? theme.palette[type].light
          : theme.palette[type].main
        : theme.palette.text.secondary,
  },

  iconWrapper: {
    marginRight: theme.spacing(0.5),
    width: theme.spacing(2),
    height: theme.spacing(2),
    lineHeight: 0,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",

    "& > svg": {
      width: theme.spacing(2),
      height: theme.spacing(2),
    },
  },
}))
