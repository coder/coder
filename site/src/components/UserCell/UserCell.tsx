import Box from "@material-ui/core/Box"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import { FC } from "react"
import { UserAvatar, UserAvatarProps } from "../UserAvatar/UserAvatar"

export interface UserCellProps {
  Avatar: UserAvatarProps
  /**
   * primaryText is rendered beside the avatar
   */
  primaryText: string /* | React.ReactNode <-- if needed */
  /**
   * caption is rendered beneath the avatar and primaryText
   */
  caption?: string /* | React.ReactNode <-- if needed */
  /**
   * onPrimaryTextSelect, if defined, is called when the primaryText is clicked
   */
  onPrimaryTextSelect?: () => void
}

const useStyles = makeStyles((theme) => ({
  primaryText: {
    color: theme.palette.text.primary,
    fontFamily: theme.typography.fontFamily,
    fontSize: "16px",
    lineHeight: "15px",
    marginBottom: "5px",
  },
}))

/**
 * UserCell is a single cell in an audit log table row that contains user-level
 * information
 */
export const UserCell: FC<React.PropsWithChildren<UserCellProps>> = ({
  Avatar,
  caption,
  primaryText,
  onPrimaryTextSelect,
}) => {
  const styles = useStyles()

  return (
    <Box alignItems="center" display="flex" flexDirection="row">
      <Box display="flex" margin="auto 14px auto 0">
        <UserAvatar {...Avatar} />
      </Box>

      <Box display="flex" flexDirection="column">
        {onPrimaryTextSelect ? (
          <Link className={styles.primaryText} onClick={onPrimaryTextSelect}>
            {primaryText}
          </Link>
        ) : (
          <Typography className={styles.primaryText}>{primaryText}</Typography>
        )}

        {caption && (
          <Typography color="textSecondary" variant="caption">
            {caption}
          </Typography>
        )}
      </Box>
    </Box>
  )
}
