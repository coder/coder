import { useOrganizationId } from "hooks/useOrganizationId";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate } from "react-router-dom";
import { CreateUserForm } from "./CreateUserForm";
import { Margins } from "components/Margins/Margins";
import { pageTitle } from "utils/page";
import { getAuthMethods } from "api/api";
import { useMutation, useQuery } from "@tanstack/react-query";
import { createUser } from "api/queries/users";
import { displaySuccess } from "components/GlobalSnackbar/utils";

export const Language = {
  unknownError: "Oops, an unknown error occurred.",
};

export const CreateUserPage: FC = () => {
  const myOrgId = useOrganizationId();
  const navigate = useNavigate();
  const createUserMutation = useMutation(createUser());
  // TODO: We should probably place this somewhere else to reduce the number of calls.
  // This would be called each time this page is loaded.
  const { data: authMethods } = useQuery({
    queryKey: ["authMethods"],
    queryFn: getAuthMethods,
  });

  return (
    <Margins>
      <Helmet>
        <title>{pageTitle("Create User")}</title>
      </Helmet>

      <CreateUserForm
        error={createUserMutation.error}
        authMethods={authMethods}
        onSubmit={async (user) => {
          await createUserMutation.mutateAsync(user);
          displaySuccess("Successfully created user.");
          navigate("/users");
        }}
        onCancel={() => {
          navigate("/users");
        }}
        isLoading={createUserMutation.isLoading}
        myOrgId={myOrgId}
      />
    </Margins>
  );
};

export default CreateUserPage;
