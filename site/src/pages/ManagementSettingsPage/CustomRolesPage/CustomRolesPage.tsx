import AddIcon from "@mui/icons-material/AddOutlined";
import Button from "@mui/material/Button";
import { getErrorMessage } from "api/errors";
import { organizationPermissions } from "api/queries/organizations";
import { deleteOrganizationRole, organizationRoles } from "api/queries/roles";
import type { Role } from "api/typesGenerated";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { type FC, useEffect, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link as RouterLink, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { useOrganizationSettings } from "../ManagementSettingsLayout";
import CustomRolesPageView from "./CustomRolesPageView";

export const CustomRolesPage: FC = () => {
  const queryClient = useQueryClient();
  const { custom_roles: isCustomRolesEnabled } = useFeatureVisibility();
  const { organization: organizationName } = useParams() as {
    organization: string;
  };
  const { organizations } = useOrganizationSettings();
  const organization = organizations?.find((o) => o.name === organizationName);
  const permissionsQuery = useQuery(organizationPermissions(organization?.id));
  const deleteRoleMutation = useMutation(
    deleteOrganizationRole(queryClient, organizationName),
  );
  const [roleToDelete, setRoleToDelete] = useState<Role>();
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
        onDeleteRole={setRoleToDelete}
        canAssignOrgRole={permissions.assignOrgRole}
        isCustomRolesEnabled={isCustomRolesEnabled}
      />

      <DeleteDialog
        key={roleToDelete?.name}
        isOpen={roleToDelete !== undefined}
        confirmLoading={deleteRoleMutation.isLoading}
        name={roleToDelete?.name ?? ""}
        entity="role"
        onCancel={() => setRoleToDelete(undefined)}
        onConfirm={async () => {
          try {
            await deleteRoleMutation.mutateAsync(roleToDelete!.name);
            setRoleToDelete(undefined);
            await organizationRolesQuery.refetch();
            displaySuccess("Custom role deleted successfully!");
          } catch (error) {
            displayError(
              getErrorMessage(error, "Failed to delete custom role"),
            );
          }
        }}
      />
    </>
  );
};

export default CustomRolesPage;
