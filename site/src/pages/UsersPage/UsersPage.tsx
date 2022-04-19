import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { XServiceContext } from "../../xServices/StateContext"
import { UsersPageView } from "./UsersPageView"

export const UsersPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [usersState] = useActor(xServices.usersXService)
  const { users, pager, getUsersError } = usersState.context

  if (usersState.matches("error")) {
    return <ErrorSummary error={getUsersError} />
  } else {
    return <UsersPageView users={users} pager={pager} />
  }
}
