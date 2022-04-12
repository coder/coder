import { makeStyles } from "@material-ui/styles"
import React from "react"
import { Pager, UserResponse } from "../../api/types"
import { Column, Table } from "../../components/Table"
import { EmptyState } from "../EmptyState"
import { Header } from "../Header"
import { UserCell } from "../Table/Cells/UserCell"

const Language = {
  usersTitle: "All users",
  emptyMessage: "No users found",
  usernameLabel: "User",
  siteRoleLabel: "Site Role"
}

const emptyState = (
  <EmptyState
    message={Language.emptyMessage}
  />
)

const columns: Column<UserResponse>[] = [
  {
    key: "email",
    name: Language.usernameLabel,
    renderer: (field, data) => {
      return <UserCell Avatar={{ username: data.email }} primaryText={data.email} />
    },
  },
  {
    key: "siteRole",
    name: Language.siteRoleLabel
  }
]

export interface UsersTableProps {
  users: UserResponse[]
  pager?: Pager
}

export const UsersTable: React.FC<UsersTableProps> = ({ users, pager }) => {
  const styles = useStyles()
  return (
    <div className={styles.flexColumn}>
      <Header title="Users" />
      <Table
        columns={columns}
        data={users}
        title={Language.usersTitle}
        emptyState={emptyState}
      />
    </div>
  )
}

const useStyles = makeStyles(() => ({
  flexColumn: {
    display: "flex",
    flexDirection: "column"
  }
}))
