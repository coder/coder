import { useMachine } from "@xstate/react";
import { useFeatureVisibility } from "hooks/useFeatureVisibility";
import { useOrganizationId } from "hooks/useOrganizationId";
import { usePermissions } from "hooks/usePermissions";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { groupsMachine } from "xServices/groups/groupsXService";
import GroupsPageView from "./GroupsPageView";

export const GroupsPage: FC = () => {
  const organizationId = useOrganizationId();
  const { createGroup: canCreateGroup } = usePermissions();
  const { template_rbac: isTemplateRBACEnabled } = useFeatureVisibility();
  const [state] = useMachine(groupsMachine, {
    context: {
      organizationId,
      shouldFetchGroups: isTemplateRBACEnabled,
    },
  });
  const { groups } = state.context;

  return (
    <>
      <Helmet>
        <title>{pageTitle("Groups")}</title>
      </Helmet>

      <GroupsPageView
        groups={groups}
        canCreateGroup={canCreateGroup}
        isTemplateRBACEnabled={isTemplateRBACEnabled}
      />
    </>
  );
};

export default GroupsPage;
