import { useMachine } from "@xstate/react"
import { User } from "api/typesGenerated"
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog"
import {
  getPaginationContext,
  nonInitialPage,
} from "components/PaginationWidget/utils"
import { useMe } from "hooks/useMe"
import { usePermissions } from "hooks/usePermissions"
import { FC, ReactNode } from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate } from "react-router"
import { useSearchParams } from "react-router-dom"
import { siteRolesMachine } from "xServices/roles/siteRolesXService"
import { usersMachine } from "xServices/users/usersXService"
import { ConfirmDialog } from "../../components/Dialogs/ConfirmDialog/ConfirmDialog"
import { ResetPasswordDialog } from "../../components/Dialogs/ResetPasswordDialog/ResetPasswordDialog"
import { pageTitle } from "../../util/page"
import { UsersPageView } from "./UsersPageView"

export const Language = {
  suspendDialogTitle: "Suspend user",
  suspendDialogAction: "Suspend",
  suspendDialogMessagePrefix: "Do you want to suspend the user",
  activateDialogTitle: "Activate user",
  activateDialogAction: "Activate",
  activateDialogMessagePrefix: "Do you want to activate the user",
}

const getSelectedUser = (id: string, users?: User[]) =>
  users?.find((u) => u.id === id)

export const UsersPage: FC<{ children?: ReactNode }> = () => {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const filter = searchParams.get("filter") ?? ""
  const [usersState, usersSend] = useMachine(usersMachine, {
    context: {
      filter,
      paginationContext: getPaginationContext(searchParams),
    },
    actions: {
      updateURL: (context, event) =>
        setSearchParams({ page: event.page, filter: context.filter }),
    },
  })
  const {
    users,
    getUsersError,
    usernameToDelete,
    usernameToSuspend,
    usernameToActivate,
    userIdToResetPassword,
    newUserPassword,
    paginationRef,
    count,
  } = usersState.context

  const { updateUsers: canEditUsers } = usePermissions()
  const [rolesState] = useMachine(siteRolesMachine, {
    context: {
      hasPermission: canEditUsers,
    },
  })
  const { roles } = rolesState.context

  // Is loading if
  // - users are loading or
  // - the user can edit the users but the roles are loading
  const isLoading =
    usersState.matches("gettingUsers") ||
    (canEditUsers && rolesState.matches("gettingRoles"))

  const me = useMe()

  return (
    <>
      <Helmet>
        <title>{pageTitle("Users")}</title>
      </Helmet>
      <UsersPageView
        roles={roles}
        users={users}
        count={count}
        onListWorkspaces={(user) => {
          navigate(
            "/workspaces?filter=" +
              encodeURIComponent(`owner:${user.username}`),
          )
        }}
        onDeleteUser={(user) => {
          usersSend({
            type: "DELETE_USER",
            userId: user.id,
            username: user.username,
          })
        }}
        onSuspendUser={(user) => {
          usersSend({
            type: "SUSPEND_USER",
            userId: user.id,
            username: user.username,
          })
        }}
        onActivateUser={(user) => {
          usersSend({
            type: "ACTIVATE_USER",
            userId: user.id,
            username: user.username,
          })
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
          usersSend({ type: "UPDATE_FILTER", query })
        }}
        paginationRef={paginationRef}
        isNonInitialPage={nonInitialPage(searchParams)}
        actorID={me.id}
      />

      <DeleteDialog
        isOpen={
          usersState.matches("confirmUserDeletion") ||
          usersState.matches("deletingUser")
        }
        confirmLoading={usersState.matches("deletingUser")}
        name={usernameToDelete ?? ""}
        entity="user"
        onConfirm={() => {
          usersSend("CONFIRM_USER_DELETE")
        }}
        onCancel={() => {
          usersSend("CANCEL_USER_DELETE")
        }}
      />

      <ConfirmDialog
        type="delete"
        hideCancel={false}
        open={
          usersState.matches("confirmUserSuspension") ||
          usersState.matches("suspendingUser")
        }
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
            {Language.suspendDialogMessagePrefix}
            {usernameToSuspend && " "}
            <strong>{usernameToSuspend ?? ""}</strong>?
          </>
        }
      />

      <ConfirmDialog
        type="success"
        hideCancel={false}
        open={
          usersState.matches("confirmUserActivation") ||
          usersState.matches("activatingUser")
        }
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
            {Language.activateDialogMessagePrefix}
            {usernameToActivate && " "}
            <strong>{usernameToActivate ?? ""}</strong>?
          </>
        }
      />

      {userIdToResetPassword && (
        <ResetPasswordDialog
          open={
            usersState.matches("confirmUserPasswordReset") ||
            usersState.matches("resettingUserPassword")
          }
          loading={usersState.matches("resettingUserPassword")}
          user={getSelectedUser(userIdToResetPassword, users)}
          newPassword={newUserPassword}
          onClose={() => {
            usersSend("CANCEL_USER_PASSWORD_RESET")
          }}
          onConfirm={() => {
            usersSend("CONFIRM_USER_PASSWORD_RESET")
          }}
        />
      )}
    </>
  )
}

export default UsersPage
