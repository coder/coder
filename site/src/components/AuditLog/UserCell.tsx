import Box from "@material-ui/core/Box"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React from "react"
import { UserAgent, UserResponse } from "../../api/types"

export const LANGUAGE = {
  emptyUser: "Deleted user",
}

export interface UserCellProps {
  onSelectEmail: () => void
  user: UserResponse
  userAgent: UserAgent
}

const useStyles = makeStyles((theme) => ({
  primaryText: {
    color: theme.palette.text.primary,
    fontSize: "16px",
    lineHeight: "15px",
    marginBottom: "5px",

    "&.MuiTypography-caption": {
      cursor: "pointer",
    },
  },
}))

export const UserCell: React.FC<UserCellProps> = ({ onSelectEmail, user, userAgent }) => {
  const styles = useStyles()

  return (
    <Box alignItems="center" display="flex" flexDirection="row">
      {/* TODO - adjust margin */}
      {/* <Box display="flex" margin="auto 14px auto 0"> */}
      {/* TODO - implement UserAvatar */}
      {/* <UserAvatar user={user} popover /> */}
      {/* </Box> */}

      <Box display="flex" flexDirection="column">
        {user.email ? (
          <Link className={styles.primaryText} onClick={onSelectEmail} variant="caption">
            {user.email}
          </Link>
        ) : (
          <Typography color="textSecondary" variant="caption">
            {LANGUAGE.emptyUser}
          </Typography>
        )}

        <Typography color="textSecondary" variant="caption">
          {userAgent.ip_address}
        </Typography>
      </Box>
    </Box>
  )
}
