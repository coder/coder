import Avatar from "@material-ui/core/Avatar"
import { FC } from "react"
import { firstLetter } from "../../util/firstLetter"

export interface UserAvatarProps {
  className?: string
  username: string
}

export const UserAvatar: FC<React.PropsWithChildren<UserAvatarProps>> = ({ username, className }) => {
  return <Avatar className={className}>{firstLetter(username)}</Avatar>
}
