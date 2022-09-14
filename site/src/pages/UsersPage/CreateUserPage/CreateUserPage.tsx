import { useMachine } from "@xstate/react"
import { useOrganizationId } from "hooks/useOrganizationId"
import React from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate } from "react-router"
import { createUserMachine } from "xServices/users/createUserXService"
import * as TypesGen from "../../../api/typesGenerated"
import { CreateUserForm } from "../../../components/CreateUserForm/CreateUserForm"
import { Margins } from "../../../components/Margins/Margins"
import { pageTitle } from "../../../util/page"

export const Language = {
  unknownError: "Oops, an unknown error occurred.",
}

export const CreateUserPage: React.FC = () => {
  const myOrgId = useOrganizationId()
  const navigate = useNavigate()
  const [usersState, usersSend] = useMachine(createUserMachine, {
    actions: {
      redirectToUsersPage: () => {
        navigate("/users")
      },
    },
  })
  const { createUserErrorMessage, createUserFormErrors } = usersState.context
  // There is no field for organization id in Community Edition, so handle its field error like a generic error
  const genericError =
    createUserErrorMessage ||
    createUserFormErrors?.organization_id ||
    (!myOrgId ? Language.unknownError : undefined)

  return (
    <Margins>
      <Helmet>
        <title>{pageTitle("Create User")}</title>
      </Helmet>
      <CreateUserForm
        formErrors={createUserFormErrors}
        onSubmit={(user: TypesGen.CreateUserRequest) => usersSend({ type: "CREATE", user })}
        onCancel={() => {
          usersSend("CANCEL_CREATE_USER")
          navigate("/users")
        }}
        isLoading={usersState.hasTag("loading")}
        error={genericError}
        myOrgId={myOrgId}
      />
    </Margins>
  )
}
