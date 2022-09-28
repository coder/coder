import { useMachine } from "@xstate/react"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useOutletContext } from "react-router-dom"
import { pageTitle } from "util/page"
import { Permissions } from "xServices/auth/authXService"
import { templateUsersMachine } from "xServices/template/templateUsersXService"
import { TemplateContext } from "xServices/template/templateXService"
import { TemplatePermissionsPageView } from "./TemplatePermissionsPageView"

export const TemplatePermissionsPage: FC<React.PropsWithChildren<unknown>> = () => {
  const { templateContext } = useOutletContext<{
    templateContext: TemplateContext
    permissions: Permissions
  }>()
  const { template, deleteTemplateError } = templateContext

  if (!template) {
    throw new Error(
      "This page should not be displayed until template, activeTemplateVersion or templateResources being loaded.",
    )
  }

  const [state, send] = useMachine(templateUsersMachine, { context: { templateId: template.id } })
  const { templateUsers, userToBeUpdated } = state.context
  const canUpdatesUsers = true

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${template.name} · Permissions`)}</title>
      </Helmet>
      <TemplatePermissionsPageView
        canUpdateUsers={canUpdatesUsers}
        templateUsers={templateUsers}
        deleteTemplateError={deleteTemplateError}
        onAddUser={(user, role, reset) => {
          send("ADD_USER", { user, role, onDone: reset })
        }}
        isAddingUser={state.matches("addingUser")}
        onUpdateUser={(user, role) => {
          send("UPDATE_USER_ROLE", { user, role })
        }}
        updatingUser={userToBeUpdated}
        onRemoveUser={(user) => {
          send("REMOVE_USER", { user })
        }}
      />
    </>
  )
}

export default TemplatePermissionsPage
