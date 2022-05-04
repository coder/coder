import { useActor } from "@xstate/react"
import React, { useContext, useEffect } from "react"
import { useNavigate } from "react-router"
import { ConfirmDialog } from "../../components/ConfirmDialog/ConfirmDialog"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
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
  const { users, getUsersError, userIdToSuspend } = usersState.context
  const navigate = useNavigate()
  const userToBeSuspended = users?.find((u) => u.id === userIdToSuspend)

  /**
   * Fetch users on component mount
   */
  useEffect(() => {
    usersSend("GET_USERS")
  }, [usersSend])

  if (!users) {
    return <FullScreenLoader />
  } else {
    return (
      <>
        <UsersPageView
          users={users}
          openUserCreationDialog={() => {
            navigate("/users/create")
          }}
          onSuspendUser={(user) => {
            usersSend({ type: "SUSPEND_USER", userId: user.id })
          }}
          error={getUsersError}
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
      </>
    )
  }
}
