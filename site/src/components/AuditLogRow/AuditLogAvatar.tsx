import Badge from "@material-ui/core/Badge"
import { withStyles } from "@material-ui/core/styles"
import { FC } from "react"
import { AuditLog } from "api/typesGenerated"
import { PaletteIndex } from "theme/palettes"
import { UserAvatar } from "components/UserAvatar/UserAvatar"

const httpStatusColor = (httpStatus: number): PaletteIndex => {
  if (httpStatus >= 300 && httpStatus < 500) {
    return "warning"
  }

  if (httpStatus >= 500) {
    return "error"
  }

  return "success"
}

interface StylesBadgeProps {
  type: PaletteIndex
}

const StyledBadge = withStyles((theme) => ({
  badge: {
    backgroundColor: ({ type }: StylesBadgeProps) => theme.palette[type].light,
    borderRadius: "100%",
    width: 10,
    minWidth: 10,
    height: 10,
    border: `2px solid ${theme.palette.background.paper}`,
    display: "block",
    padding: 0,
  },
}))(Badge)

const StyledUserAvatar = withStyles((theme) => ({
  root: {
    background: theme.palette.divider,
    color: theme.palette.text.primary,
    border: `2px solid ${theme.palette.divider}`,

    "& svg": {
      width: 18,
      height: 18,
    },
  },
}))(UserAvatar)

export type AuditLogAvatarProps = {
  auditLog: AuditLog
}

export const AuditLogAvatar: FC<AuditLogAvatarProps> = ({ auditLog }) => {
  return (
    <StyledBadge
      role="status"
      type={httpStatusColor(auditLog.status_code)}
      arial-label={auditLog.status_code}
      title={auditLog.status_code.toString()}
      overlap="circular"
      anchorOrigin={{
        vertical: "bottom",
        horizontal: "right",
      }}
      badgeContent={<div></div>}
    >
      <StyledUserAvatar
        username={auditLog.user?.username ?? ""}
        avatarURL={auditLog.user?.avatar_url}
      />
    </StyledBadge>
  )
}
