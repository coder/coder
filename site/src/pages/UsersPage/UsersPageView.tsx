import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { Pager, UserResponse } from "../../api/types"
import { Header } from "../../components/Header/Header"
import { UsersTable } from "../../components/UsersTable/UsersTable"

export const Language = {
  pageTitle: "Users",
  pageSubtitle: (pager: Pager | undefined): string => (pager ? `${pager.total} total` : ""),
}

export interface UsersPageViewProps {
  users: UserResponse[]
  pager?: Pager
}

export const UsersPageView: React.FC<UsersPageViewProps> = ({ users, pager }) => {
  const styles = useStyles()
  return (
    <div className={styles.flexColumn}>
      <Header title={Language.pageTitle} subTitle={Language.pageSubtitle(pager)} />
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
