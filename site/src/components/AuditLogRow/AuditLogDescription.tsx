import { FC } from "react"
import { AuditLog } from "api/typesGenerated"
import { Link as RouterLink } from "react-router-dom"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import { useTranslation } from "react-i18next"
import { BuildAuditDescription } from "./BuildAuditDescription"

export const AuditLogDescription: FC<{ auditLog: AuditLog }> = ({
  auditLog,
}): JSX.Element => {
  const classes = useStyles()
  const { t } = useTranslation("auditLog")

  const target = auditLog.resource_target.trim()
  const user = auditLog.user
    ? auditLog.user.username.trim()
    : t("table.logRow.unknownUser")

  if (auditLog.resource_type === "workspace_build") {
    return <BuildAuditDescription auditLog={auditLog} />
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

  // return (
  //   <span>
  //     <Trans
  //       t={t}
  //       i18nKey="table.logRow.auditDescription"
  //       values={{ truncatedDescription, target }}
  //     >
  //       {"{{truncatedDescription}}"}
  //       <Link component={RouterLink} to={auditLog.resource_link}>
  //         <strong>{"{{target}}"}</strong>
  //       </Link>
  //     </Trans>
  //   </span>
  // )

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
          <>{t("table.logRow.deletedLabel")}</>
        </span>
      )}
      {/* logs for workspaces created on behalf of other users indicate ownership in the description */}
      {auditLog.additional_fields.workspace_owner &&
        auditLog.additional_fields.workspace_owner !== "unknown" &&
        auditLog.additional_fields.workspace_owner !==
          auditLog.user?.username && (
          <span>
            <>
              {t("table.logRow.onBehalfOf", {
                owner: auditLog.additional_fields.workspace_owner,
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
