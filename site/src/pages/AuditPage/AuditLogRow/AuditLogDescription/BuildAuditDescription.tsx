import type { AuditLog } from "api/typesGenerated";
import { type FC, useMemo } from "react";
import { Link as RouterLink } from "react-router-dom";
import Link from "@mui/material/Link";

interface BuildAuditDescriptionProps {
  auditLog: AuditLog;
}

export const BuildAuditDescription: FC<BuildAuditDescriptionProps> = ({
  auditLog,
}) => {
  const workspaceName = auditLog.additional_fields?.workspace_name?.trim();
  // workspaces can be started/stopped/deleted by a user, or kicked off automatically by Coder
  const user =
    auditLog.additional_fields?.build_reason &&
    auditLog.additional_fields?.build_reason !== "initiator"
      ? "Coder automatically"
      : auditLog.user?.username.trim();

  const action = useMemo(() => {
    switch (auditLog.action) {
      case "start":
        return "started";
      case "stop":
        return "stopped";
      case "delete":
        return "deleted";
      default:
        return auditLog.action;
    }
  }, [auditLog.action]);

  return (
    <span>
      {user} <strong>{action}</strong> workspace{" "}
      {auditLog.resource_link ? (
        <Link component={RouterLink} to={auditLog.resource_link}>
          <strong>{workspaceName}</strong>
        </Link>
      ) : (
        <strong>{workspaceName}</strong>
      )}
    </span>
  );
};
