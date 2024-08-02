import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import { getErrorMessage } from "api/errors";
import { patchOrganizationRole, organizationRoles } from "api/queries/roles";
import { displayError } from "components/GlobalSnackbar/utils";
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
  const { data } = useQuery(organizationRoles(organization));
  const role = data?.find((role) => role.name === roleName);
  const pageTitleText =
    role !== undefined ? "Edit Custom Role" : "Create Custom Role";

  return (
    <>
      <Helmet>
        <title>{pageTitle(pageTitleText)}</title>
      </Helmet>
      <CreateEditRolePageView
        role={role}
        organization={organization}
        onSubmit={async (data) => {
          try {
            console.log({ data });
            await patchOrganizationRoleMutation.mutateAsync(data);
            navigate(`/organizations/${organization}/roles`);
          } catch (error) {
            console.log({ error });
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
