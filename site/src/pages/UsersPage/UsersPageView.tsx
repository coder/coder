import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { Pager, UserResponse } from "../../api/types"
import { Header } from "../../components/Header"
import { UsersTable } from "../../components/UsersTable/UsersTable"

const Language = {
  pageTitle: "Users",
}

export interface UsersPageViewProps {
  users: UserResponse[]
  pager?: Pager
}

export const UsersPageView: React.FC<UsersPageViewProps> = ({ users, pager }) => {
  const styles = useStyles()
  return (
    <div className={styles.flexColumn}>
      <Header title={Language.pageTitle} subTitle={pager ? `${pager.total} total` : ""} />
      <UsersTable users={users} />
    </div>
  )
}

const useStyles = makeStyles(() => ({
  flexColumn: {
    display: "flex",
    flexDirection: "column",
  },
}))
