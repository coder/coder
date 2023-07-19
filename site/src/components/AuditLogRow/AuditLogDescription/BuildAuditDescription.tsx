import { Trans, useTranslation } from "react-i18next"
import { AuditLog } from "api/typesGenerated"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import Link from "@mui/material/Link"

export const BuildAuditDescription: FC<{ auditLog: AuditLog }> = ({
  auditLog,
}): JSX.Element => {
  const { t } = useTranslation("auditLog")

  const workspaceName = auditLog.additional_fields?.workspace_name?.trim()
  // workspaces can be started/stopped/deleted by a user, or kicked off automatically by Coder
  const user =
    auditLog.additional_fields?.build_reason &&
    auditLog.additional_fields?.build_reason !== "initiator"
      ? "Coder automatically"
      : auditLog.user?.username.trim()

  const action: string = (() => {
    switch (auditLog.action) {
      case "start":
        return "started"
      case "stop":
        return "stopped"
      case "delete":
        return "deleted"
      default:
        return auditLog.action
    }
  })()

  if (auditLog.resource_link) {
    return (
      <span>
        <Trans
          t={t}
          i18nKey="table.logRow.description.linkedWorkspaceBuild"
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

  return (
    <span>
      <Trans
        t={t}
        i18nKey="table.logRow.description.unlinkedWorkspaceBuild"
        values={{ user, action, workspaceName }}
      >
        {"{{user}}"}
        {"{{action}}"}workspace{"{{workspaceName}}"}
      </Trans>
    </span>
  )
}
