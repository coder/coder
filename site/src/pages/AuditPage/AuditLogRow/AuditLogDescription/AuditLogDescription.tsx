import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import type { AuditLog } from "#/api/typesGenerated";
import { Link } from "#/components/Link/Link";
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
	let user = auditLog.user
		? auditLog.user.username.trim()
		: "Unauthenticated user";

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

	const description =
		getAuditLogDescriptionOverride(auditLog) ?? auditLog.description;
	const truncatedDescription = description
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
				<Link asChild showExternalIcon={false} className="text-base px-0">
					<RouterLink to={auditLog.resource_link}>
						<strong>{target}</strong>
					</RouterLink>
				</Link>
			) : (
				<strong>{target}</strong>
			)}
			{onBehalfOf}
		</span>
	);
};

/**
 * Returns a semantic description override for successful chat write operations,
 * or undefined to fall through to the backend description. Derives archive,
 * unarchive, and sharing descriptions from the audit diff so they apply to
 * historical logs without backfilling.
 */
export const getAuditLogDescriptionOverride = (
	auditLog: AuditLog,
): string | undefined => {
	if (
		auditLog.resource_type !== "chat" ||
		auditLog.action !== "write" ||
		auditLog.status_code >= 400 ||
		// 303 See Other: the backend treats redirects as failed attempts, not successful writes.
		auditLog.status_code === 303
	) {
		return undefined;
	}

	const diffEntries = Object.entries(auditLog.diff);
	if (diffEntries.length === 1) {
		const [fieldName, diff] = diffEntries[0];
		if (fieldName === "archived") {
			if (diff.old === false && diff.new === true) {
				return "{user} archived chat {target}";
			}
			if (diff.old === true && diff.new === false) {
				return "{user} unarchived chat {target}";
			}
		}
	}

	if (
		diffEntries.length > 0 &&
		diffEntries.every(
			([fieldName]) => fieldName === "user_acl" || fieldName === "group_acl",
		)
	) {
		return "{user} updated sharing for chat {target}";
	}

	return undefined;
};

function AppSessionAuditLogDescription({ auditLog }: AuditLogDescriptionProps) {
	const { connection_type, workspace_owner, workspace_name } =
		auditLog.additional_fields;

	return (
		<>
			{connection_type} session to {workspace_owner}'s{" "}
			<Link asChild showExternalIcon={false} className="text-base px-0">
				<RouterLink to={`${auditLog.resource_link}`}>
					<strong>{workspace_name}</strong>
				</RouterLink>
			</Link>{" "}
			workspace{" "}
			<strong>{auditLog.action === "disconnect" ? "closed" : "opened"}</strong>
		</>
	);
}
