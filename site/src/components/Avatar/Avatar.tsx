// This is the only place MuiAvatar can be used
// eslint-disable-next-line no-restricted-imports -- Read above
import MuiAvatar, {
  AvatarProps as MuiAvatarProps,
} from "@material-ui/core/Avatar"
import { makeStyles } from "@material-ui/core/styles"
import { cloneElement, FC } from "react"
import { combineClasses } from "util/combineClasses"
import { firstLetter } from "./firstLetter"

export type AvatarProps = MuiAvatarProps & {
  size?: "sm" | "md" | "xl"
  colorScheme?: "light" | "darken"
  fitImage?: boolean
}

export const Avatar: FC<AvatarProps> = ({
  size = "md",
  colorScheme = "light",
  fitImage,
  className,
  children,
  ...muiProps
}) => {
  const styles = useStyles()

  return (
    <MuiAvatar
      {...muiProps}
      className={combineClasses([
        className,
        styles[size],
        styles[colorScheme],
        fitImage && styles.fitImage,
      ])}
    >
      {/* If the children is a string, we always want to render the first letter */}
      {typeof children === "string" ? firstLetter(children) : children}
    </MuiAvatar>
  )
}

export const AvatarIcon: FC<{ children: JSX.Element }> = ({ children }) => {
  const styles = useStyles()
  return cloneElement(children, { className: styles.avatarIcon })
}

const useStyles = makeStyles((theme) => ({
  // Size styles
  sm: {
    width: theme.spacing(4),
    height: theme.spacing(4),
    fontSize: theme.spacing(2),
  },
  // Just use the default value from theme
  md: {},
  xl: {
    width: theme.spacing(6),
    height: theme.spacing(6),
    fontSize: theme.spacing(3),
  },
  // Colors
  // Just use the default value from theme
  light: {},
  darken: {
    background: theme.palette.divider,
    color: theme.palette.text.primary,
  },
  // Avatar icon
  avatarIcon: {
    maxWidth: "50%",
  },
  // Fit image
  fitImage: {
    "& .MuiAvatar-img": {
      objectFit: "contain",
    },
  },
}))
