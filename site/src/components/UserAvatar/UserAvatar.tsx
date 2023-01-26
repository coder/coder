import { Avatar } from "components/Avatar/Avatar"
import { FC } from "react"

export interface UserAvatarProps {
  username: string
  avatarURL?: string
  // It is needed to work with the AvatarGroup so it can pass the
  // MuiAvatarGroup-avatar className
  className?: string
}

export const UserAvatar: FC<UserAvatarProps> = ({
  username,
  avatarURL,
  className,
}) => {
  return (
    <Avatar title={username} src={avatarURL} className={className}>
      {username}
    </Avatar>
  )
}
