import { useActor } from "@xstate/react"
import React, { useContext, useEffect } from "react"
import { Helmet } from "react-helmet"
import { useNavigate } from "react-router"
import { useSearchParams } from "react-router-dom"
import { ConfirmDialog } from "../../components/ConfirmDialog/ConfirmDialog"
import { ResetPasswordDialog } from "../../components/ResetPasswordDialog/ResetPasswordDialog"
import { userFilterQuery } from "../../util/filters"
import { pageTitle } from "../../util/page"
import { XServiceContext } from "../../xServices/StateContext"
import { UsersPageView } from "./UsersPageView"

export const Language = {
  suspendDialogTitle: "Suspend user",
  suspendDialogAction: "Suspend",
  suspendDialogMessagePrefix: "Do you want to suspend the user",
  activateDialogTitle: "Activate user",
  activateDialogAction: "Activate",
  activateDialogMessagePrefix: "Do you want to activate the user",
}

export const UsersPage: React.FC<React.PropsWithChildren<unknown>> = () => {
  const xServices = useContext(XServiceContext)
  const [usersState, usersSend] = useActor(xServices.usersXService)
  const {
    users,
    getUsersError,
    userIdToSuspend,
    userIdToActivate,
    userIdToResetPassword,
    newUserPassword,
  } = usersState.context
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const userToBeSuspended = users?.find((u) => u.id === userIdToSuspend)
  const userToBeActivated = users?.find((u) => u.id === userIdToActivate)
  const userToResetPassword = users?.find((u) => u.id === userIdToResetPassword)

  const [authState, _] = useActor(xServices.authXService)
  const { permissions } = authState.context
  const canEditUsers = permissions && permissions.updateUsers
  const canCreateUser = permissions && permissions.createUser

  const [rolesState, rolesSend] = useActor(xServices.siteRolesXService)
  const { roles } = rolesState.context

  // Is loading if
  // - permissions are loading or
  // - users are loading or
  // - the user can edit the users but the roles are loading
  const isLoading =
    authState.matches("gettingPermissions") ||
    usersState.matches("gettingUsers") ||
    (canEditUsers && rolesState.matches("gettingRoles"))

  // Fetch users on component mount
  useEffect(() => {
    const filter = searchParams.get("filter")
    const query = filter ?? userFilterQuery.active
    usersSend({
      type: "GET_USERS",
      query,
    })
  }, [searchParams, usersSend])

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
      <Helmet>
        <title>{pageTitle("Users")}</title>
      </Helmet>
      <UsersPageView
        roles={roles}
        users={users}
        openUserCreationDialog={() => {
          navigate("/users/create")
        }}
        onSuspendUser={(user) => {
          usersSend({ type: "SUSPEND_USER", userId: user.id })
        }}
        onActivateUser={(user) => {
          usersSend({ type: "ACTIVATE_USER", userId: user.id })
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
        filter={usersState.context.filter}
        onFilter={(query) => {
          searchParams.set("filter", query)
          setSearchParams(searchParams)
        }}
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

      <ConfirmDialog
        type="success"
        hideCancel={false}
        open={usersState.matches("confirmUserActivation")}
        confirmLoading={usersState.matches("activatingUser")}
        title={Language.activateDialogTitle}
        confirmText={Language.activateDialogAction}
        onConfirm={() => {
          usersSend("CONFIRM_USER_ACTIVATION")
        }}
        onClose={() => {
          usersSend("CANCEL_USER_ACTIVATION")
        }}
        description={
          <>
            {Language.activateDialogMessagePrefix} <strong>{userToBeActivated?.username}</strong>?
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
