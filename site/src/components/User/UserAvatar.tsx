import Avatar from "@material-ui/core/Avatar"
import React from "react"
import { UserResponse } from "../../api/types"

export interface UserAvatarProps {
  user: UserResponse
  className?: string
}

export const UserAvatar: React.FC<UserAvatarProps> = ({ user, className }) => {
  return <Avatar className={className}>{firstLetter(user.username)}</Avatar>
}

/**
 * `firstLetter` extracts the first character and returns it, uppercased
 *
 * If the string is empty or null, returns an empty string
 */
export const firstLetter = (str: string): string => {
  if (str && str.length > 0) {
    return str[0].toLocaleUpperCase()
  }

  return ""
}
