import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import React from "react"
import * as TypesGen from "../../api/typesGenerated"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { UsersTable } from "../../components/UsersTable/UsersTable"

export const Language = {
  pageTitle: "Users",
  createButton: "New user",
}

export interface UsersPageViewProps {
  users?: TypesGen.User[]
  roles?: TypesGen.Role[]
  error?: unknown
  isUpdatingUserRoles?: boolean
  canEditUsers?: boolean
  canCreateUser?: boolean
  isLoading?: boolean
  openUserCreationDialog: () => void
  onSuspendUser: (user: TypesGen.User) => void
  onResetUserPassword: (user: TypesGen.User) => void
  onUpdateUserRoles: (user: TypesGen.User, roles: TypesGen.Role["name"][]) => void
}

export const UsersPageView: React.FC<UsersPageViewProps> = ({
  users,
  roles,
  openUserCreationDialog,
  onSuspendUser,
  onResetUserPassword,
  onUpdateUserRoles,
  error,
  isUpdatingUserRoles,
  canEditUsers,
  canCreateUser,
  isLoading,
}) => {
  const styles = useStyles()

  return (
    <Stack spacing={4}>
      <Margins>
        <div className={styles.actions}>
          <div>
            {canCreateUser && (
              <Button onClick={openUserCreationDialog} startIcon={<AddCircleOutline />}>
                {Language.createButton}
              </Button>
            )}
          </div>
        </div>
        {error ? (
          <ErrorSummary error={error} />
        ) : (
          <UsersTable
            users={users}
            roles={roles}
            onSuspendUser={onSuspendUser}
            onResetUserPassword={onResetUserPassword}
            onUpdateUserRoles={onUpdateUserRoles}
            isUpdatingUserRoles={isUpdatingUserRoles}
            canEditUsers={canEditUsers}
            isLoading={isLoading}
          />
        )}
      </Margins>
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  actions: {
    marginTop: theme.spacing(3),
    marginBottom: theme.spacing(3),
    display: "flex",
    height: theme.spacing(6),

    "& > *": {
      marginLeft: "auto",
    },
  },
}))
