import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import { getErrorMessage } from "api/errors";
import { organizationPermissions } from "api/queries/organizations";
import { patchOrganizationRole, organizationRoles } from "api/queries/roles";
import type { PatchRoleRequest } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { pageTitle } from "utils/page";
import { useOrganizationSettings } from "../ManagementSettingsLayout";
import CreateEditRolePageView from "./CreateEditRolePageView";

export const CreateEditRolePage: FC = () => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { organization: organizationName, roleName } = useParams() as {
    organization: string;
    roleName: string;
  };
  const { organizations } = useOrganizationSettings();
  const organization = organizations?.find((o) => o.name === organizationName);
  const permissionsQuery = useQuery(organizationPermissions(organization?.id));
  const patchOrganizationRoleMutation = useMutation(
    patchOrganizationRole(queryClient, organizationName),
  );
  const { data: roleData, isLoading } = useQuery(
    organizationRoles(organizationName),
  );
  const role = roleData?.find((role) => role.name === roleName);
  const permissions = permissionsQuery.data;

  if (isLoading || !permissions) {
    return <Loader />;
  }

  return (
    <>
      <Helmet>
        <title>
          {pageTitle(
            role !== undefined ? "Edit Custom Role" : "Create Custom Role",
          )}
        </title>
      </Helmet>

      <CreateEditRolePageView
        role={role}
        onSubmit={async (data: PatchRoleRequest) => {
          try {
            await patchOrganizationRoleMutation.mutateAsync(data);
            navigate(`/organizations/${organizationName}/roles`);
          } catch (error) {
            displayError(
              getErrorMessage(error, "Failed to update custom role"),
            );
          }
        }}
        error={patchOrganizationRoleMutation.error}
        isLoading={patchOrganizationRoleMutation.isLoading}
        organizationName={organizationName}
        canAssignOrgRole={permissions.assignOrgRole}
      />
    </>
  );
};

export default CreateEditRolePage;
