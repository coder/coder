import { type FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { getErrorMessage } from "api/errors";
import { groups } from "api/queries/groups";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { usePermissions } from "contexts/auth/usePermissions";
import { useFeatureVisibility } from "hooks/useFeatureVisibility";
import { pageTitle } from "utils/page";
import { displayError } from "components/GlobalSnackbar/utils";
import GroupsPageView from "./GroupsPageView";

export const GroupsPage: FC = () => {
  const organizationId = useOrganizationId();
  const { createGroup: canCreateGroup } = usePermissions();
  const { template_rbac: isTemplateRBACEnabled } = useFeatureVisibility();
  const groupsQuery = useQuery(groups(organizationId));

  useEffect(() => {
    if (groupsQuery.error) {
      displayError(
        getErrorMessage(groupsQuery.error, "Error on loading groups."),
      );
    }
  }, [groupsQuery.error]);

  return (
    <>
      <Helmet>
        <title>{pageTitle("Groups")}</title>
      </Helmet>

      <GroupsPageView
        groups={groupsQuery.data}
        canCreateGroup={canCreateGroup}
        isTemplateRBACEnabled={isTemplateRBACEnabled}
      />
    </>
  );
};

export default GroupsPage;
