import AddIcon from "@mui/icons-material/AddOutlined";
import Button from "@mui/material/Button";
import { type FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { Link as RouterLink, useParams } from "react-router-dom";
import { getErrorMessage } from "api/errors";
import { organizationPermissions } from "api/queries/organizations";
import { organizationRoles } from "api/queries/roles";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { pageTitle } from "utils/page";
import { useOrganizationSettings } from "../ManagementSettingsLayout";
import CustomRolesPageView from "./CustomRolesPageView";

export const CustomRolesPage: FC = () => {
  const { custom_roles: isCustomRolesEnabled } = useFeatureVisibility();
  const { organization: organizationName } = useParams() as {
    organization: string;
  };
  const { organizations } = useOrganizationSettings();
  const organization = organizations?.find((o) => o.name === organizationName);
  const permissionsQuery = useQuery(organizationPermissions(organization?.id));
  const organizationRolesQuery = useQuery(organizationRoles(organizationName));
  const filteredRoleData = organizationRolesQuery.data?.filter(
    (role) => role.built_in === false,
  );
  const permissions = permissionsQuery.data;

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

  if (!permissions) {
    return <Loader />;
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle("Custom Roles")}</title>
      </Helmet>

      <Stack
        alignItems="baseline"
        direction="row"
        justifyContent="space-between"
      >
        <SettingsHeader
          title="Custom Roles"
          description="Manage custom roles for this organization."
        />
        {permissions.assignOrgRole && isCustomRolesEnabled && (
          <Button component={RouterLink} startIcon={<AddIcon />} to="create">
            Create custom role
          </Button>
        )}
      </Stack>

      <CustomRolesPageView
        roles={filteredRoleData}
        canAssignOrgRole={permissions.assignOrgRole}
        isCustomRolesEnabled={isCustomRolesEnabled}
      />
    </>
  );
};

export default CustomRolesPage;
