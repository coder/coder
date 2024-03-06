import Link from "@mui/material/Link";
import type { FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import type { AuditLog } from "api/typesGenerated";
import { BuildAuditDescription } from "./BuildAuditDescription";

interface AuditLogDescriptionProps {
  auditLog: AuditLog;
}

export const AuditLogDescription: FC<AuditLogDescriptionProps> = ({
  auditLog,
}) => {
  let target = auditLog.resource_target.trim();
  const user = auditLog.user?.username.trim();

  if (auditLog.resource_type === "workspace_build") {
    return <BuildAuditDescription auditLog={auditLog} />;
  }

  // SSH key entries have no links
  if (auditLog.resource_type === "git_ssh_key") {
    target = "";
  }

  const truncatedDescription = auditLog.description
    .replace("{user}", `${user}`)
    .replace("{target}", "");

  // logs for workspaces created on behalf of other users indicate ownership in the description
  const onBehalfOf =
    auditLog.additional_fields.workspace_owner &&
    auditLog.additional_fields.workspace_owner !== "unknown" &&
    auditLog.additional_fields.workspace_owner.trim() !== user
      ? ` on behalf of ${auditLog.additional_fields.workspace_owner}`
      : "";

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
      {onBehalfOf}
    </span>
  );
};
