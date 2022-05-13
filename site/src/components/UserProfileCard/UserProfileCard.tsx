import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React from "react"
import * as TypesGen from "../../api/typesGenerated"
import { UserAvatar } from "../UserAvatar/UserAvatar"

interface UserProfileCardProps {
  user: TypesGen.User
}

export const UserProfileCard: React.FC<UserProfileCardProps> = ({ user }) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <div className={styles.avatarContainer}>
        <UserAvatar className={styles.avatar} username={user.username} />
      </div>
      <Typography className={styles.userName}>{user.username}</Typography>
      <Typography className={styles.userEmail}>{user.email}</Typography>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    paddingTop: theme.spacing(3),
    textAlign: "center",
  },
  avatarContainer: {
    width: "100%",
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
  },
  avatar: {
    width: 48,
    height: 48,
    borderRadius: "50%",
    marginBottom: theme.spacing(1),
    transition: `transform .2s`,

    "&:hover": {
      transform: `scale(1.1)`,
    },
  },
  userName: {
    fontSize: 16,
    marginBottom: theme.spacing(0.5),
  },
  userEmail: {
    fontSize: 14,
    letterSpacing: 0.2,
    color: theme.palette.text.secondary,
    marginBottom: theme.spacing(1.5),
  },
}))
