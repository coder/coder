import Avatar from "@material-ui/core/Avatar"
import React from "react"
import { firstLetter } from "../../util/firstLetter"

export interface UserAvatarProps {
  className?: string
  username: string
}

export const UserAvatar: React.FC<UserAvatarProps> = ({ username, className }) => {
  return <Avatar className={className}>{firstLetter(username)}</Avatar>
}
