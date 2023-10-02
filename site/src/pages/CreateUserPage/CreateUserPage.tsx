import { useOrganizationId } from "hooks/useOrganizationId";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate } from "react-router-dom";
import { CreateUserForm } from "./CreateUserForm";
import { Margins } from "components/Margins/Margins";
import { pageTitle } from "utils/page";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { authMethods, createUser } from "api/queries/users";
import { displaySuccess } from "components/GlobalSnackbar/utils";

export const Language = {
  unknownError: "Oops, an unknown error occurred.",
};

export const CreateUserPage: FC = () => {
  const myOrgId = useOrganizationId();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const createUserMutation = useMutation(createUser(queryClient));
  const authMethodsQuery = useQuery(authMethods());

  return (
    <Margins>
      <Helmet>
        <title>{pageTitle("Create User")}</title>
      </Helmet>

      <CreateUserForm
        error={createUserMutation.error}
        authMethods={authMethodsQuery.data}
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
