import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
import { authMethods, createUser } from "api/queries/users";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Margins } from "components/Margins/Margins";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { pageTitle } from "utils/page";
import { CreateUserForm } from "./CreateUserForm";

export const Language = {
  unknownError: "Oops, an unknown error occurred.",
};

export const CreateUserPage: FC = () => {
  const { organizationId } = useAuthenticated();
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
        organizationId={organizationId}
      />
    </Margins>
  );
};

export default CreateUserPage;
