import { FC } from "react"
import { AuditLog } from "api/typesGenerated"
import { Link as RouterLink } from "react-router-dom"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import i18next from "i18next"

export const AuditLogDescription: FC<{ auditLog: AuditLog }> = ({
  auditLog,
}): JSX.Element => {
  const classes = useStyles()
  const { t } = i18next

  let target = auditLog.resource_target.trim()
  let user = auditLog.user?.username.trim()

  if (auditLog.resource_type === "workspace_build") {
    // audit logs with a resource_type of workspace build use workspace name as a target
    target = auditLog.additional_fields.workspaceName.trim()
    // workspaces can be started/stopped by a user, or kicked off automatically by Coder
    user =
      auditLog.additional_fields.buildReason &&
      auditLog.additional_fields.buildReason !== "initiator"
        ? t("auditLog:table.logRow.buildReason")
        : auditLog.user?.username.trim()
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
    .replace("{user}", `${user}`)
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
      {auditLog.is_deleted && (
        <span className={classes.deletedLabel}>
          <>{t("auditLog:table.logRow.deletedLabel")}</>
        </span>
      )}
      {/* logs for workspaces created on behalf of other users indicate ownership in the description */}
      {auditLog.additional_fields.workspaceOwner &&
        auditLog.additional_fields.workspaceOwner !== "unknown" && (
          <span>
            <>
              {t("auditLog:table.logRow.onBehalfOf", {
                owner: auditLog.additional_fields.workspaceOwner,
              })}
            </>
          </span>
        )}
    </span>
  )
}

const useStyles = makeStyles((theme) => ({
  deletedLabel: {
    ...theme.typography.caption,
    color: theme.palette.text.secondary,
  },
}))
