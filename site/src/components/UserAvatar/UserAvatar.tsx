import { Avatar } from "components/Avatar/Avatar"
import { FC } from "react"

export interface UserAvatarProps {
  username: string
  avatarURL?: string
}

export const UserAvatar: FC<UserAvatarProps> = ({ username, avatarURL }) => {
  return (
    <Avatar title={username} src={avatarURL}>
      {username}
    </Avatar>
  )
}
