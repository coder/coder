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

interface ComponentWithPaletteIndex {
  paletteIndex: PaletteIndex
}

const StyledBadge = withStyles((theme) => ({
  badge: {
    backgroundColor: ({ paletteIndex }: ComponentWithPaletteIndex) =>
      theme.palette[paletteIndex].light,
    borderRadius: "100%",
    width: 10,
    minWidth: 10,
    height: 10,
    border: `2px solid ${theme.palette.background.paper}`,
    display: "block",
    padding: 0,
  },
}))(Badge)

export type AuditLogAvatarProps = {
  auditLog: AuditLog
}

export const AuditLogAvatar: FC<AuditLogAvatarProps> = ({ auditLog }) => {
  const paletteIndex = httpStatusColor(auditLog.status_code)

  return (
    <StyledBadge
      role="status"
      paletteIndex={paletteIndex}
      arial-label={auditLog.status_code}
      title={auditLog.status_code.toString()}
      overlap="circular"
      anchorOrigin={{
        vertical: "bottom",
        horizontal: "right",
      }}
      badgeContent={<div></div>}
    >
      <UserAvatar
        username={auditLog.user?.username ?? ""}
        avatarURL={auditLog.user?.avatar_url}
      />
    </StyledBadge>
  )
}
