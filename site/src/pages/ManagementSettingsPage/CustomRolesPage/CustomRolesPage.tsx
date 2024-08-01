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
import { organizationRoles } from "api/queries/roles";
import type { Organization } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { pageTitle } from "utils/page";
import CustomRolesPageView from "./CustomRolesPageView";

export const CustomRolesPage: FC = () => {
  const { permissions } = useAuthenticated();
  const { assignOrgRole: canAssignOrgRole } = permissions;
  const {
    multiple_organizations: organizationsEnabled,
    custom_roles: isCustomRolesEnabled,
  } = useFeatureVisibility();
  const { experiments } = useDashboard();
  const location = useLocation();
  const { organization = "default" } = useParams() as { organization: string };
  const organizationRolesQuery = useQuery(organizationRoles(organization));

  useEffect(() => {
    if (organizationRolesQuery.error) {
      displayError(
        getErrorMessage(
          organizationRolesQuery.error,
          "Error loading custom roles.",
        ),
      );
    }
  }, [organizationRolesQuery.error]);

  // if (
  //   organizationsEnabled &&
  //   experiments.includes("multi-organization") &&
  //   location.pathname === "/deployment/groups"
  // ) {
  //   const defaultName =
  //     getOrganizationNameByDefault(organizations) ?? "default";
  //   return <Navigate to={`/organizations/${defaultName}/groups`} replace />;
  // }

  return (
    <>
      <Helmet>
        <title>{pageTitle("Groups")}</title>
      </Helmet>

      <PageHeader
        actions={
          <>
            {canAssignOrgRole && isCustomRolesEnabled && (
              <Button
                component={RouterLink}
                startIcon={<GroupAdd />}
                to="create"
              >
                Create custom role
              </Button>
            )}
          </>
        }
      >
        <PageHeaderTitle>Custom Roles</PageHeaderTitle>
      </PageHeader>

      <CustomRolesPageView
        roles={organizationRolesQuery.data}
        canAssignOrgRole={canAssignOrgRole}
        isCustomRolesEnabled={isCustomRolesEnabled}
      />
    </>
  );
};

export default CustomRolesPage;
