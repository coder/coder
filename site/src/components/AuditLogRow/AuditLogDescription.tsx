import { FC } from "react"
import { AuditLog } from "api/typesGenerated"
import { Link as RouterLink } from "react-router-dom"
import Link from "@material-ui/core/Link"

export const AuditLogDescription: FC<{ auditLog: AuditLog }> = ({
  auditLog,
}): JSX.Element => {
  let target = auditLog.resource_target.trim()

  // audit logs with a resource_type of workspace build use workspace name as a target
  if (
    auditLog.resource_type === "workspace_build" &&
    auditLog.additional_fields.workspaceName
  ) {
    target = auditLog.additional_fields.workspaceName.trim()
  }

  // SSH key entries have no links
  if (auditLog.resource_type === "git_ssh_key") {
    return (
      <span>
        {auditLog.description
          .replace("{user}", `${auditLog.user?.username.trim()}`)
          .replace("{target}", `${target}`)}
      </span>
    )
  }

  const truncatedDescription = auditLog.description
    .replace("{user}", `${auditLog.user?.username.trim()}`)
    .replace("{target}", "")

  return (
    <span>
      {truncatedDescription}
      {auditLog.resource_link ? (
        <Link component={RouterLink} to={auditLog.resource_link}>
          <strong>{target}</strong>
        </Link>
      ) : (
        <strong>{target}</strong>
      )}
    </span>
  )
}
