import Link from "@mui/material/Link";
import type { AuditLog } from "api/typesGenerated";
import type { FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import { BuildAuditDescription } from "./BuildAuditDescription";

interface AuditLogDescriptionProps {
	auditLog: AuditLog;
}

export const AuditLogDescription: FC<AuditLogDescriptionProps> = ({
	auditLog,
}) => {
	if (auditLog.resource_type === "workspace_build") {
		return <BuildAuditDescription auditLog={auditLog} />;
	}
	if (auditLog.additional_fields?.connection_type) {
		return <AppSessionAuditLogDescription auditLog={auditLog} />;
	}

	let target = auditLog.resource_target.trim();
	let user = auditLog.user?.username.trim();

	// SSH key entries have no links
	if (auditLog.resource_type === "git_ssh_key") {
		target = "";
	}

	// This occurs when SCIM creates a user, or dormancy changes a users status.
	if (
		auditLog.resource_type === "user" &&
		auditLog.additional_fields?.automatic_actor === "coder"
	) {
		user = "Coder automatically";
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

function AppSessionAuditLogDescription({ auditLog }: AuditLogDescriptionProps) {
	const { connection_type, workspace_owner, workspace_name } =
		auditLog.additional_fields;

	return (
		<>
			{connection_type} session to {workspace_owner}'s{" "}
			<Link component={RouterLink} to={`@${workspace_owner}/${workspace_name}`}>
				<strong>{workspace_name}</strong>
			</Link>{" "}
			workspace{" "}
			<strong>{auditLog.action === "disconnect" ? "closed" : "opened"}</strong>
		</>
	);
}
