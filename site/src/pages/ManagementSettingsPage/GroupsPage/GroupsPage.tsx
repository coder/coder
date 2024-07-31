import GroupAdd from "@mui/icons-material/GroupAddOutlined";
import Button from "@mui/material/Button";
import { type FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { Navigate, Link as RouterLink, useParams } from "react-router-dom";
import { getErrorMessage } from "api/errors";
import { groups } from "api/queries/groups";
import { organizationPermissions } from "api/queries/organizations";
import type { Organization } from "api/typesGenerated";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { pageTitle } from "utils/page";
import { useOrganizationSettings } from "../ManagementSettingsLayout";
import GroupsPageView from "./GroupsPageView";

export const GroupsPage: FC = () => {
  const feats = useFeatureVisibility();
  const { organization: organizationName } = useParams() as {
    organization: string;
  };
  const groupsQuery = useQuery(
    organizationName ? groups(organizationName) : { enabled: false },
  );
  const { organizations } = useOrganizationSettings();
  // TODO: If we could query permissions based on the name then we would not
  //       have to cascade off the organizations query.
  const organization = organizations?.find((o) => o.name === organizationName);
  const permissionsQuery = useQuery(organizationPermissions(organization?.id));

  useEffect(() => {
    if (groupsQuery.error) {
      displayError(
        getErrorMessage(groupsQuery.error, "Unable to load groups."),
      );
    }
  }, [groupsQuery.error]);

  useEffect(() => {
    if (permissionsQuery.error) {
      displayError(
        getErrorMessage(permissionsQuery.error, "Unable to load permissions."),
      );
    }
  }, [permissionsQuery.error]);

  if (!organizations) {
    return <Loader />;
  }

  if (!organizationName) {
    const defaultName = getOrganizationNameByDefault(organizations);
    if (defaultName) {
      return <Navigate to={`/organizations/${defaultName}/groups`} replace />;
    }
    return <EmptyState message="No default organization found" />;
  }

  const permissions = permissionsQuery.data;
  if (!permissions) {
    return <Loader />;
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle("Groups")}</title>
      </Helmet>

      <PageHeader
        actions={
          <>
            {permissions.createGroup && feats.template_rbac && (
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
        canCreateGroup={permissions.createGroup}
        isTemplateRBACEnabled={feats.template_rbac}
      />
    </>
  );
};

export default GroupsPage;

export const getOrganizationNameByDefault = (organizations: Organization[]) =>
  organizations.find((org) => org.is_default)?.name;
