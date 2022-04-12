import React from "react"
import { UserResponse } from "../../api/types"
import { Column, Table } from "../../components/Table"
import { EmptyState } from "../EmptyState"
import { UserCell } from "../Table/Cells/UserCell"

const Language = {
  pageTitle: "Users",
  usersTitle: "All users",
  emptyMessage: "No users found",
  usernameLabel: "User",
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
}

export const UsersTable: React.FC<UsersTableProps> = ({ users }) => {
  return <Table columns={columns} data={users} title={Language.usersTitle} emptyState={emptyState} />
}
