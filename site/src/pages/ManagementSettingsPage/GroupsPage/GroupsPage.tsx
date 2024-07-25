import GroupAdd from "@mui/icons-material/GroupAddOutlined";
import Button from "@mui/material/Button";
import { type FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { Link as RouterLink, useParams } from "react-router-dom";
import { getErrorMessage } from "api/errors";
import { groups } from "api/queries/groups";
import { displayError } from "components/GlobalSnackbar/utils";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { pageTitle } from "utils/page";
import GroupsPageView from "./GroupsPageView";

export const GroupsPage: FC = () => {
  const { permissions } = useAuthenticated();
  const { createGroup: canCreateGroup } = permissions;
  const { template_rbac: isTemplateRBACEnabled } = useFeatureVisibility();
  const { organization = "default" } = useParams() as { organization: string };
  const groupsQuery = useQuery(groups(organization));

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

      <PageHeader
        actions={
          <>
            {canCreateGroup && isTemplateRBACEnabled && (
              <Button
                component={RouterLink}
                startIcon={<GroupAdd />}
                to="create"
              >
                Create group
              </Button>
            )}
          </>
        }
      >
        <PageHeaderTitle>Groups</PageHeaderTitle>
      </PageHeader>

      <GroupsPageView
        groups={groupsQuery.data}
        canCreateGroup={canCreateGroup}
        isTemplateRBACEnabled={isTemplateRBACEnabled}
      />
    </>
  );
};

export default GroupsPage;
