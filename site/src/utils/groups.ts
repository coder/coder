import type { Group } from "api/typesGenerated";

/**
 * Returns true if the provided group is the 'Everyone' group.
 * The everyone group represents all the users in an organization
 * for which every organization member is implicitly a member of.
 *
 * @param {Group} group - The group to evaluate.
 * @returns {boolean} - Returns true if the group's ID matches its
 * organization ID.
 */
export const isEveryoneGroup = (group: Group): boolean =>
  group.id === group.organization_id;

export const getGroupSubtitle = (group: Group): string => {
  // It is the everyone group when a group id is the same of the org id
  if (group.id === group.organization_id) {
    return `All users`;
  }

  if (!group.members) {
    return `0 members`;
  }

  if (group.members.length === 1) {
    return `1 member`;
  }

  return `${group.members.length} members`;
};
