import Avatar from "@material-ui/core/Avatar"
import { makeStyles } from "@material-ui/core/styles"
import { User } from "api/typesGenerated"
import { FC } from "react"
import { firstLetter } from "../../util/firstLetter"

export const AutocompleteAvatar: FC<{ user: User }> = ({ user }) => {
  const styles = useStyles()

  return (
    <div className={styles.avatarContainer}>
      {user.avatar_url ? (
        <img className={styles.avatar} alt={`${user.username}'s Avatar`} src={user.avatar_url} />
      ) : (
        <Avatar>{firstLetter(user.username)}</Avatar>
      )}
    </div>
  )
}

export const useStyles = makeStyles((theme) => {
  return {
    avatarContainer: {
      margin: "0px 10px",
    },
    avatar: {
      width: theme.spacing(4.5),
      height: theme.spacing(4.5),
      borderRadius: "100%",
    },
  }
})
