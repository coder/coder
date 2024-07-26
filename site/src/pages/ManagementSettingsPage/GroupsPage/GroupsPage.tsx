import GroupAdd from "@mui/icons-material/GroupAddOutlined";
import Button from "@mui/material/Button";
import { type FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import {
  Navigate,
  Link as RouterLink,
  useLocation,
  useParams,
} from "react-router-dom";
import { getErrorMessage } from "api/errors";
import { groups } from "api/queries/groups";
import type { Organization } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { pageTitle } from "utils/page";
import { useOrganizationSettings } from "../ManagementSettingsLayout";
import GroupsPageView from "./GroupsPageView";

export const GroupsPage: FC = () => {
  const { permissions } = useAuthenticated();
  const { createGroup: canCreateGroup } = permissions;
  const {
    multiple_organizations: organizationsEnabled,
    template_rbac: isTemplateRBACEnabled,
  } = useFeatureVisibility();
  const { experiments } = useDashboard();
  const location = useLocation();
  const { organization = "default" } = useParams() as { organization: string };
  const groupsQuery = useQuery(groups(organization));
  const { organizations } = useOrganizationSettings();

  useEffect(() => {
    if (groupsQuery.error) {
      displayError(
        getErrorMessage(groupsQuery.error, "Error on loading groups."),
      );
    }
  }, [groupsQuery.error]);

  if (
    organizationsEnabled &&
    experiments.includes("multi-organization") &&
    location.pathname === "/deployment/groups"
  ) {
    const defaultName =
      getOrganizationNameByDefault(organizations) ?? "default";
    return <Navigate to={`/organizations/${defaultName}/groups`} replace />;
  }

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

export const getOrganizationNameByDefault = (organizations: Organization[]) =>
  organizations.find((org) => org.is_default)?.name;
