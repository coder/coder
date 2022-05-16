import Avatar from "@material-ui/core/Avatar"
import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { combineClasses } from "../../util/combineClasses"
import { firstLetter } from "../../util/firstLetter"

export interface UserAvatarProps {
  className?: string
  username: string
}

export const UserAvatar: React.FC<UserAvatarProps> = ({ username, className }) => {
  const styles = useStyles()
  return (
    <Avatar variant="square" className={combineClasses([styles.avatar, className])}>
      {firstLetter(username)}
    </Avatar>
  )
}

const useStyles = makeStyles((theme) => ({
  avatar: {
    borderRadius: 2,
    border: `1px solid ${theme.palette.divider}`,
  },
}))
