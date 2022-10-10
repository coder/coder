import Avatar from "@material-ui/core/Avatar"
import { FC } from "react"
import { firstLetter } from "../../util/firstLetter"

export interface UserAvatarProps {
  username: string
  className?: string
  avatarURL?: string
}

export const UserAvatar: FC<UserAvatarProps> = ({
  username,
  className,
  avatarURL,
}) => {
  return (
    <Avatar className={className} title={username}>
      {avatarURL ? (
        <img alt={`${username}'s Avatar`} src={avatarURL} width="100%" />
      ) : (
        firstLetter(username)
      )}
    </Avatar>
  )
}
