import { useMachine } from "@xstate/react"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useOutletContext } from "react-router-dom"
import { pageTitle } from "util/page"
import { templateUsersMachine } from "xServices/template/templateUsersXService"
import { TemplateContext } from "xServices/template/templateXService"
import { TemplateCollaboratorsPageView } from "./TemplateCollaboratorsPageView"

export const TemplateCollaboratorsPage: FC<React.PropsWithChildren<unknown>> = () => {
  const { template, activeTemplateVersion, templateResources, deleteTemplateError } =
    useOutletContext<TemplateContext>()

  if (!template || !activeTemplateVersion || !templateResources) {
    throw new Error(
      "This page should not be displayed until template, activeTemplateVersion or templateResources being loaded.",
    )
  }

  const [state, send] = useMachine(templateUsersMachine, { context: { templateId: template.id } })
  const { templateUsers } = state.context

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${template.name} · Collaborators`)}</title>
      </Helmet>
      <TemplateCollaboratorsPageView
        templateUsers={templateUsers}
        deleteTemplateError={deleteTemplateError}
        onAddUser={(user, role, reset) => {
          send("ADD_USER", { user, role, onDone: reset })
        }}
        isAddingUser={state.matches("addingUser")}
      />
    </>
  )
}

export default TemplateCollaboratorsPage
