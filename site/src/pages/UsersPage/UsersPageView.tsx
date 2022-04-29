import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { Pager, UserResponse } from "../../api/types"
import { Header } from "../../components/Header/Header"
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
  const styles = useStyles()

  return (
    <div className={styles.flexColumn}>
      <Header
        title={Language.pageTitle}
        subTitle={Language.pageSubtitle(pager)}
        action={{ text: Language.newUserButton, onClick: openUserCreationDialog }}
      />
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
