import { useMachine } from "@xstate/react";
import { useOrganizationId } from "hooks/useOrganizationId";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate } from "react-router-dom";
import { createUserMachine } from "xServices/users/createUserXService";
import * as TypesGen from "api/typesGenerated";
import { CreateUserForm } from "./CreateUserForm";
import { Margins } from "components/Margins/Margins";
import { pageTitle } from "utils/page";
import { getAuthMethods } from "api/api";
import { useQuery } from "@tanstack/react-query";

export const Language = {
  unknownError: "Oops, an unknown error occurred.",
};

export const CreateUserPage: FC = () => {
  const myOrgId = useOrganizationId();
  const navigate = useNavigate();
  const [createUserState, createUserSend] = useMachine(createUserMachine, {
    actions: {
      redirectToUsersPage: () => {
        navigate("/users");
      },
    },
  });
  const { error } = createUserState.context;

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
        error={error}
        authMethods={authMethods}
        onSubmit={(user: TypesGen.CreateUserRequest) =>
          createUserSend({ type: "CREATE", user })
        }
        onCancel={() => {
          createUserSend("CANCEL_CREATE_USER");
          navigate("/users");
        }}
        isLoading={createUserState.hasTag("loading")}
        myOrgId={myOrgId}
      />
    </Margins>
  );
};

export default CreateUserPage;
