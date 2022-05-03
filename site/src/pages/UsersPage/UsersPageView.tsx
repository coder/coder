import Paper from "@material-ui/core/Paper"
import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { UserResponse } from "../../api/types"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { Header } from "../../components/Header/Header"
import { UsersTable } from "../../components/UsersTable/UsersTable"

export const Language = {
  pageTitle: "Users",
  newUserButton: "New User",
}

export interface UsersPageViewProps {
  users: UserResponse[]
  openUserCreationDialog: () => void
  onSuspendUser: (user: UserResponse) => void
  error?: unknown
}

export const UsersPageView: React.FC<UsersPageViewProps> = ({
  users,
  openUserCreationDialog,
  onSuspendUser,
  error,
}) => {
  const styles = useStyles()

  return (
    <div className={styles.flexColumn}>
      <Header title={Language.pageTitle} action={{ text: Language.newUserButton, onClick: openUserCreationDialog }} />
      <Paper style={{ maxWidth: "1380px", margin: "1em auto", width: "100%" }}>
        {error ? <ErrorSummary error={error} /> : <UsersTable users={users} onSuspendUser={onSuspendUser} />}
      </Paper>
    </div>
  )
}

const useStyles = makeStyles(() => ({
  flexColumn: {
    display: "flex",
    flexDirection: "column",
  },
}))
