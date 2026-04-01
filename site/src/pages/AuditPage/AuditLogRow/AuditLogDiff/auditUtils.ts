import type { AuditDiff } from "api/typesGenerated";

interface GroupMember {
	user_id: string;
	group_id: string;
}

/**
 *
 * @param auditLogDiff
 * @returns a diff with the 'members' key flattened to be an array of user_ids
 */
export const determineGroupDiff = (auditLogDiff: AuditDiff): AuditDiff => {
	const old = auditLogDiff.members?.old as GroupMember[] | undefined;
	const new_ = auditLogDiff.members?.new as GroupMember[] | undefined;

	return {
		...auditLogDiff,
		members: {
			old: old?.map((groupMember) => groupMember.user_id),
			new: new_?.map((groupMember) => groupMember.user_id),
			secret: auditLogDiff.members?.secret,
		},
	};
};

/**
 *
 * @param auditLogDiff
 * @returns a diff with the 'mappings' as a JSON string. Otherwise, it is [Object object]
 */
export const determineIdPSyncMappingDiff = (
	auditLogDiff: AuditDiff,
): AuditDiff => {
	const old = auditLogDiff.mapping?.old as Record<string, string[]> | undefined;
	const new_ = auditLogDiff.mapping?.new as
		| Record<string, string[]>
		| undefined;
	if (!old || !new_) {
		return auditLogDiff;
	}

	return {
		...auditLogDiff,
		mapping: {
			old: JSON.stringify(old),
			new: JSON.stringify(new_),
			secret: auditLogDiff.mapping?.secret,
		},
	};
};
