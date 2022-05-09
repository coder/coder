import React from "react"
import { UserResponse } from "../../api/types"
import { EmptyState } from "../EmptyState/EmptyState"
import { Column, Table } from "../Table/Table"
import { TableRowMenu } from "../TableRowMenu/TableRowMenu"
import { UserCell } from "../UserCell/UserCell"

export const Language = {
  pageTitle: "Users",
  usersTitle: "All users",
  emptyMessage: "No users found",
  usernameLabel: "User",
  suspendMenuItem: "Suspend",
  resetPasswordMenuItem: "Reset password",
}

const emptyState = <EmptyState message={Language.emptyMessage} />

const columns: Column<UserResponse>[] = [
  {
    key: "username",
    name: Language.usernameLabel,
    renderer: (field, data) => {
      return <UserCell Avatar={{ username: data.username }} primaryText={data.username} caption={data.email} />
    },
  },
]

export interface UsersTableProps {
  users: UserResponse[]
  onSuspendUser: (user: UserResponse) => void
  onResetUserPassword: (user: UserResponse) => void
}

export const UsersTable: React.FC<UsersTableProps> = ({ users, onSuspendUser, onResetUserPassword }) => {
  return (
    <Table
      columns={columns}
      data={users}
      title={Language.usersTitle}
      emptyState={emptyState}
      rowMenu={(user) => (
        <TableRowMenu
          data={user}
          menuItems={[
            {
              label: Language.suspendMenuItem,
              onClick: onSuspendUser,
            },
            {
              label: Language.resetPasswordMenuItem,
              onClick: onResetUserPassword,
            },
          ]}
        />
      )}
    />
  )
}
