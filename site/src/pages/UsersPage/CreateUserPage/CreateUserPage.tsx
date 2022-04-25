import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { useNavigate } from "react-router"
import { CreateUserRequest } from "../../../api/typesGenerated"
import { CreateUserForm } from "../../../components/CreateUserForm/CreateUserForm"
import { XServiceContext } from "../../../xServices/StateContext"

export const CreateUserPage = () => {
  const xServices = useContext(XServiceContext)
  const [_, usersSend] = useActor(xServices.usersXService)
  const navigate = useNavigate()

  return <CreateUserForm onSubmit={(user: CreateUserRequest) => usersSend({ type: "CREATE", user })} onCancel={() => navigate("/users")} />
}
