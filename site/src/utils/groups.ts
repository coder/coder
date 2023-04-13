import { Group } from "api/typesGenerated"

export const everyOneGroup = (organizationId: string): Group => ({
  id: organizationId,
  name: "Everyone",
  organization_id: organizationId,
  members: [],
  avatar_url: "",
  quota_allowance: 0,
})

export const getGroupSubtitle = (group: Group): string => {
  // It is the everyone group when a group id is the same of the org id
  if (group.id === group.organization_id) {
    return `All users`
  }

  if (group.members.length === 1) {
    return `1 member`
  }

  return `${group.members.length} members`
}
