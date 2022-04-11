import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { ErrorSummary } from "../../components/ErrorSummary"
import { UsersTable } from "../../components/UsersTable/UsersTable"
import { XServiceContext } from "../../xServices/StateContext"

export type Role = "Admin" | "Member"

export interface User {
  username: string
  email: string
  siteRole: Role
}

export const UsersPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [usersState] = useActor(xServices.usersXService)
  const { users, getUsersError } = usersState.context

  if (usersState.matches("error")) {
    return <ErrorSummary error={getUsersError} />
  } else {
    return <UsersTable users={users} />
  }
}
