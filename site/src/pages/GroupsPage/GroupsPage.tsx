import { useMachine } from "@xstate/react"
import { useFeatureVisibility } from "hooks/useFeatureVisibility"
import { useOrganizationId } from "hooks/useOrganizationId"
import { usePermissions } from "hooks/usePermissions"
import React from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { groupsMachine } from "xServices/groups/groupsXService"
import GroupsPageView from "./GroupsPageView"

export const GroupsPage: React.FC = () => {
  const organizationId = useOrganizationId()
  const [state] = useMachine(groupsMachine, {
    context: {
      organizationId,
    },
  })
  const { groups } = state.context
  const { createGroup: canCreateGroup } = usePermissions()
  const { rbac: isRBACEnabled } = useFeatureVisibility()

  return (
    <>
      <Helmet>
        <title>{pageTitle("Groups")}</title>
      </Helmet>

      <GroupsPageView
        groups={groups}
        canCreateGroup={canCreateGroup}
        isRBACEnabled={isRBACEnabled}
      />
    </>
  )
}

export default GroupsPage
