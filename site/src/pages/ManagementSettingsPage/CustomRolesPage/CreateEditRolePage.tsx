import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import { getErrorMessage } from "api/errors";
import { patchOrganizationRole, organizationRoles } from "api/queries/roles";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { pageTitle } from "utils/page";
import CreateEditRolePageView from "./CreateEditRolePageView";

export const CreateGroupPage: FC = () => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { organization, roleName } = useParams() as {
    organization: string;
    roleName: string;
  };
  const patchOrganizationRoleMutation = useMutation(
    patchOrganizationRole(queryClient, organization ?? "default"),
  );
  const { data: roleData, isLoading } = useQuery(
    organizationRoles(organization),
  );
  const role = roleData?.find((role) => role.name === roleName);

  if (isLoading) {
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
        organization={organization}
        onSubmit={async (data) => {
          try {
            await patchOrganizationRoleMutation.mutateAsync(data);
            navigate(`/organizations/${organization}/roles`);
          } catch (error) {
            displayError(
              getErrorMessage(error, "Failed to update custom role"),
            );
          }
        }}
        error={patchOrganizationRoleMutation.error}
        isLoading={patchOrganizationRoleMutation.isLoading}
      />
    </>
  );
};

export default CreateGroupPage;
