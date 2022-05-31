import Chip from "@material-ui/core/Chip"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { Role } from "../../api/typesGenerated"
import { UserAvatar } from "../UserAvatar/UserAvatar"

export interface UserProfileCardProps {
  user: TypesGen.User
}

export const UserProfileCard: FC<UserProfileCardProps> = ({ user }) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <div className={styles.avatarContainer}>
        <UserAvatar className={styles.avatar} username={user.username} />
      </div>
      <Typography className={styles.userName}>{user.username}</Typography>
      <Typography className={styles.userEmail}>{user.email}</Typography>
      <ul className={styles.chipContainer}>
        {user.roles.map((role: Role) => (
          <li key={role.name} className={styles.chipStyles}>
            <Chip classes={{ root: styles.chipRoot }} label={role.display_name} />
          </li>
        ))}
      </ul>
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
  },
  chipContainer: {
    display: "flex",
    justifyContent: "center",
    flexWrap: "wrap",
    listStyle: "none",
    margin: "0",
    padding: `${theme.spacing(1.5)}px ${theme.spacing(2.75)}px`,
  },
  chipStyles: {
    margin: theme.spacing(0.5),
  },
  chipRoot: {
    backgroundColor: "#7057FF",
  },
}))
