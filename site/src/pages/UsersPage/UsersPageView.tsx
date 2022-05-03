import React from "react"
import { UserResponse } from "../../api/types"
import { Header } from "../../components/Header/Header"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { UsersTable } from "../../components/UsersTable/UsersTable"

export const Language = {
  pageTitle: "Users",
  newUserButton: "New User",
}

export interface UsersPageViewProps {
  users: UserResponse[]
  openUserCreationDialog: () => void
}

export const UsersPageView: React.FC<UsersPageViewProps> = ({ users, openUserCreationDialog }) => {
  return (
    <Stack spacing={4}>
      <Header title={Language.pageTitle} action={{ text: Language.newUserButton, onClick: openUserCreationDialog }} />
      <Margins>
        <UsersTable users={users} />
      </Margins>
    </Stack>
  )
}
