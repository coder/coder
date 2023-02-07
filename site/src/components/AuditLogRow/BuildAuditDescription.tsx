import { Trans, useTranslation } from "react-i18next"
import { AuditLog } from "api/typesGenerated"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import Link from "@material-ui/core/Link"

export const BuildAuditDescription: FC<{ auditLog: AuditLog }> = ({
  auditLog,
}): JSX.Element => {
  const { t } = useTranslation("auditLog")

  // audit logs with a resource_type of workspace build use workspace name as a target
  const workspaceName = auditLog.additional_fields?.workspace_name?.trim()
  // workspaces can be started/stopped by a user, or kicked off automatically by Coder
  const user =
    auditLog.additional_fields?.build_reason &&
    auditLog.additional_fields?.build_reason !== "initiator"
      ? "Coder automatically"
      : auditLog.user?.username.trim()

  const action = auditLog.action === "start" ? "started" : "stopped"

  return (
    <span>
      <Trans
        t={t}
        i18nKey="table.logRow.workspaceBuild"
        values={{ user, action, workspaceName }}
      >
        {"{{user}}"}
        <Link component={RouterLink} to={auditLog.resource_link}>
          {"{{action}}"}
        </Link>
        workspace{"{{workspaceName}}"}
      </Trans>
    </span>
  )
}
