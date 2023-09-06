import { AuditDiff } from "api/typesGenerated";

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
  return {
    ...auditLogDiff,
    members: {
      old: auditLogDiff.members?.old?.map(
        (groupMember: GroupMember) => groupMember.user_id,
      ),
      new: auditLogDiff.members?.new?.map(
        (groupMember: GroupMember) => groupMember.user_id,
      ),
      secret: auditLogDiff.members?.secret,
    },
  };
};
