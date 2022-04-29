import React from "react"
import { Pager, UserResponse } from "../../api/types"
import { Header } from "../../components/Header/Header"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { UsersTable } from "../../components/UsersTable/UsersTable"

export const Language = {
  pageTitle: "Users",
  pageSubtitle: (pager: Pager | undefined): string => (pager ? `${pager.total} total` : ""),
  newUserButton: "New User",
}

export interface UsersPageViewProps {
  users: UserResponse[]
  pager?: Pager
  openUserCreationDialog: () => void
}

export const UsersPageView: React.FC<UsersPageViewProps> = ({ users, pager, openUserCreationDialog }) => {
  return (
    <Stack spacing={4}>
      <Header
        title={Language.pageTitle}
        subTitle={Language.pageSubtitle(pager)}
        action={{ text: Language.newUserButton, onClick: openUserCreationDialog }}
      />
      <Margins>
        <UsersTable users={users} />
      </Margins>
    </Stack>
  )
}
