import { useActor, useSelector } from "@xstate/react"
import React, { useContext } from "react"
import { useNavigate } from "react-router"
import * as TypesGen from "../../../api/typesGenerated"
import { CreateUserForm } from "../../../components/CreateUserForm/CreateUserForm"
import { Margins } from "../../../components/Margins/Margins"
import { selectOrgId } from "../../../xServices/auth/authSelectors"
import { XServiceContext } from "../../../xServices/StateContext"

export const Language = {
  unknownError: "Oops, an unknown error occurred.",
}

export const CreateUserPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const myOrgId = useSelector(xServices.authXService, selectOrgId)
  const [usersState, usersSend] = useActor(xServices.usersXService)
  const { createUserError, createUserFormErrors } = usersState.context
  const navigate = useNavigate()
  // There is no field for organization id in Community Edition, so handle its field error like a generic error
  const genericError =
    createUserError || createUserFormErrors?.organization_id || !myOrgId ? Language.unknownError : undefined

  return (
    <Margins>
      <CreateUserForm
        formErrors={createUserFormErrors}
        onSubmit={(user: TypesGen.CreateUserRequest) => usersSend({ type: "CREATE", user })}
        onCancel={() => navigate("/users")}
        isLoading={usersState.hasTag("loading")}
        error={genericError}
        myOrgId={myOrgId ?? ""}
      />
    </Margins>
  )
}
