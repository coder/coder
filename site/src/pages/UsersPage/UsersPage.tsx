import { useActor, useSelector } from "@xstate/react"
import React, { useContext, useEffect } from "react"
import { useNavigate } from "react-router"
import { ConfirmDialog } from "../../components/ConfirmDialog/ConfirmDialog"
import { ResetPasswordDialog } from "../../components/ResetPasswordDialog/ResetPasswordDialog"
import { selectPermissions } from "../../xServices/auth/authSelectors"
import { XServiceContext } from "../../xServices/StateContext"
import { UsersPageView } from "./UsersPageView"

export const Language = {
  suspendDialogTitle: "Suspend user",
  suspendDialogAction: "Suspend",
  suspendDialogMessagePrefix: "Do you want to suspend the user",
}

export const UsersPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [usersState, usersSend] = useActor(xServices.usersXService)
  const [rolesState, rolesSend] = useActor(xServices.siteRolesXService)
  const { users, getUsersError, userIdToSuspend, userIdToResetPassword, newUserPassword } = usersState.context
  const navigate = useNavigate()
  const userToBeSuspended = users?.find((u) => u.id === userIdToSuspend)
  const userToResetPassword = users?.find((u) => u.id === userIdToResetPassword)
  const permissions = useSelector(xServices.authXService, selectPermissions)
  const canEditUsers = permissions && permissions.updateUsers
  const canCreateUser = permissions && permissions.createUser
  const { roles } = rolesState.context
  // Is loading if
  // - permissions are not loaded or
  // - users are not loaded or
  // - the user can edit the users but the roles are not loaded yet
  const isLoading = !permissions || !users || (canEditUsers && !roles)

  // Fetch users on component mount
  useEffect(() => {
    usersSend("GET_USERS")
  }, [usersSend])

  // Fetch roles on component mount
  useEffect(() => {
    // Only fetch the roles if the user has permission for it
    if (canEditUsers) {
      rolesSend({
        type: "GET_ROLES",
      })
    }
  }, [canEditUsers, rolesSend])

  return (
    <>
      <UsersPageView
        roles={roles}
        users={users}
        openUserCreationDialog={() => {
          navigate("/users/create")
        }}
        onSuspendUser={(user) => {
          usersSend({ type: "SUSPEND_USER", userId: user.id })
        }}
        onResetUserPassword={(user) => {
          usersSend({ type: "RESET_USER_PASSWORD", userId: user.id })
        }}
        onUpdateUserRoles={(user, roles) => {
          usersSend({
            type: "UPDATE_USER_ROLES",
            userId: user.id,
            roles,
          })
        }}
        error={getUsersError}
        isUpdatingUserRoles={usersState.matches("updatingUserRoles")}
        isLoading={isLoading}
        canEditUsers={canEditUsers}
        canCreateUser={canCreateUser}
      />

      <ConfirmDialog
        type="delete"
        hideCancel={false}
        open={usersState.matches("confirmUserSuspension")}
        confirmLoading={usersState.matches("suspendingUser")}
        title={Language.suspendDialogTitle}
        confirmText={Language.suspendDialogAction}
        onConfirm={() => {
          usersSend("CONFIRM_USER_SUSPENSION")
        }}
        onClose={() => {
          usersSend("CANCEL_USER_SUSPENSION")
        }}
        description={
          <>
            {Language.suspendDialogMessagePrefix} <strong>{userToBeSuspended?.username}</strong>?
          </>
        }
      />

      <ResetPasswordDialog
        loading={usersState.matches("resettingUserPassword")}
        user={userToResetPassword}
        newPassword={newUserPassword}
        open={usersState.matches("confirmUserPasswordReset")}
        onClose={() => {
          usersSend("CANCEL_USER_PASSWORD_RESET")
        }}
        onConfirm={() => {
          usersSend("CONFIRM_USER_PASSWORD_RESET")
        }}
      />
    </>
  )
}
