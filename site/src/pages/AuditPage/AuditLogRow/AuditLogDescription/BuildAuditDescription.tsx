import type { AuditLog } from "api/typesGenerated";
import { Link } from "components/Link/Link";
import { type FC, useMemo } from "react";
import { Link as RouterLink } from "react-router";
import { systemBuildReasons } from "utils/workspace";

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
		systemBuildReasons.includes(auditLog.additional_fields?.build_reason)
			? "Coder automatically"
			: auditLog.user
				? auditLog.user.username.trim()
				: "Unauthenticated user";

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
				<Link asChild showExternalIcon={false} className="text-base px-0">
					<RouterLink to={auditLog.resource_link}>
						<strong>{workspaceName}</strong>
					</RouterLink>
				</Link>
			) : (
				<strong>{workspaceName}</strong>
			)}
		</span>
	);
};
