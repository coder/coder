import { useActor } from "@xstate/react"
import React, { useContext, useEffect } from "react"
import { Navigate } from "react-router"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { XServiceContext } from "../../xServices/StateContext"
import { UsersPageView } from "./UsersPageView"

export const UsersPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [usersState, usersSend] = useActor(xServices.usersXService)
  const { users, pager, getUsersError } = usersState.context

  /**
   * Fetch users on component mount
   */
  useEffect(() => {
    usersSend("GET_USERS")
  }, [])

  if (usersState.matches("error")) {
    return <ErrorSummary error={getUsersError} />
  } else if (usersState.matches("creationMode")) {
    return <Navigate to="/users/create" />
  } else {
    return (
      <UsersPageView
        users={users}
        pager={pager}
        openUserCreationDialog={() => {
          usersSend("ENTER_CREATION_MODE")
        }}
      />
    )
  }
}
