import GroupAdd from "@mui/icons-material/GroupAddOutlined";
import Button from "@mui/material/Button";
import { type FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { Link as RouterLink, useParams } from "react-router-dom";
import { getErrorMessage } from "api/errors";
import { organizationRoles } from "api/queries/roles";
import { displayError } from "components/GlobalSnackbar/utils";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { pageTitle } from "utils/page";
import CustomRolesPageView from "./CustomRolesPageView";

export const CustomRolesPage: FC = () => {
  const { permissions } = useAuthenticated();
  const { assignOrgRole: canAssignOrgRole } = permissions;
  const { custom_roles: isCustomRolesEnabled } = useFeatureVisibility();

  const { organization = "default" } = useParams() as { organization: string };
  const organizationRolesQuery = useQuery(organizationRoles(organization));
  const filteredRoleData = organizationRolesQuery.data?.filter(
    (role) => role.built_in === false,
  );

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

  return (
    <>
      <Helmet>
        <title>{pageTitle("Custom Roles")}</title>
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
        roles={filteredRoleData}
        canAssignOrgRole={canAssignOrgRole}
        isCustomRolesEnabled={isCustomRolesEnabled}
      />
    </>
  );
};

export default CustomRolesPage;
