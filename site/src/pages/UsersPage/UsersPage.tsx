import { useActor, useMachine } from "@xstate/react"
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog"
import { usePermissions } from "hooks/usePermissions"
import { FC, ReactNode, useContext, useEffect } from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate } from "react-router"
import { useSearchParams } from "react-router-dom"
import { usersMachine } from "xServices/users/usersXService"
import { ConfirmDialog } from "../../components/Dialogs/ConfirmDialog/ConfirmDialog"
import { ResetPasswordDialog } from "../../components/Dialogs/ResetPasswordDialog/ResetPasswordDialog"
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

export const UsersPage: FC<{ children?: ReactNode }> = () => {
  const xServices = useContext(XServiceContext)
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const filter = searchParams.get("filter") ?? undefined
  const [usersState, usersSend] = useMachine(usersMachine, {
    context: {
      filter,
    },
  })
  const {
    users,
    getUsersError,
    userIdToDelete,
    userIdToSuspend,
    userIdToActivate,
    userIdToResetPassword,
    newUserPassword,
  } = usersState.context

  const userToBeSuspended = users?.find((u) => u.id === userIdToSuspend)
  const userToBeDeleted = users?.find((u) => u.id === userIdToDelete)
  const userToBeActivated = users?.find((u) => u.id === userIdToActivate)
  const userToResetPassword = users?.find((u) => u.id === userIdToResetPassword)
  const { updateUsers: canEditUsers } = usePermissions()
  const [rolesState, rolesSend] = useActor(xServices.siteRolesXService)
  const { roles } = rolesState.context

  // Is loading if
  // - users are loading or
  // - the user can edit the users but the roles are loading
  const isLoading =
    usersState.matches("gettingUsers") ||
    (canEditUsers && rolesState.matches("gettingRoles"))

  // Fetch users on component mount
  useEffect(() => {
    usersSend({
      type: "GET_USERS",
      query: filter,
    })
  }, [filter, usersSend])

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
        onListWorkspaces={(user) => {
          navigate(
            "/workspaces?filter=" +
              encodeURIComponent(`owner:${user.username}`),
          )
        }}
        onDeleteUser={(user) => {
          usersSend({ type: "DELETE_USER", userId: user.id })
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
        filter={usersState.context.filter}
        onFilter={(query) => {
          searchParams.set("filter", query)
          setSearchParams(searchParams)
        }}
      />

      {userToBeDeleted && (
        <DeleteDialog
          isOpen={usersState.matches("confirmUserDeletion")}
          confirmLoading={usersState.matches("deletingUser")}
          name={userToBeDeleted.username}
          entity="user"
          onConfirm={() => {
            usersSend("CONFIRM_USER_DELETE")
          }}
          onCancel={() => {
            usersSend("CANCEL_USER_DELETE")
          }}
        />
      )}

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
            {Language.suspendDialogMessagePrefix}{" "}
            <strong>{userToBeSuspended?.username}</strong>?
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
            {Language.activateDialogMessagePrefix}{" "}
            <strong>{userToBeActivated?.username}</strong>?
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

export default UsersPage
