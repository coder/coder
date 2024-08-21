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
