import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import { patchOrganizationRole } from "api/queries/roles";
import { pageTitle } from "utils/page";
import CreateEditRolePageView from "./CreateEditRolePageView";

export const CreateGroupPage: FC = () => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { organization } = useParams() as { organization: string };
  const patchOrganizationRoleMutation = useMutation(
    patchOrganizationRole(queryClient, organization ?? "default"),
  );

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create Custom Role")}</title>
      </Helmet>
      <CreateEditRolePageView
        onSubmit={async (data) => {
          const newRole = await patchOrganizationRoleMutation.mutateAsync(data);
          console.log({ newRole });
          navigate(`/organizations/${organization}/roles`);
        }}
        error={patchOrganizationRoleMutation.error}
        isLoading={patchOrganizationRoleMutation.isLoading}
      />
    </>
  );
};

export default CreateGroupPage;
