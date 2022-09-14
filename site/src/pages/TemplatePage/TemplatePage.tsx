import { useMachine, useSelector } from "@xstate/react"
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog"
import { FC, useContext } from "react"
import { Helmet } from "react-helmet-async"
import { Navigate, useParams } from "react-router-dom"
import { selectPermissions } from "xServices/auth/authSelectors"
import { XServiceContext } from "xServices/StateContext"
import { Loader } from "../../components/Loader/Loader"
import { useOrganizationId } from "../../hooks/useOrganizationId"
import { pageTitle } from "../../util/page"
import { templateMachine } from "../../xServices/template/templateXService"
import { TemplatePageView } from "./TemplatePageView"

const useTemplateName = () => {
  const { template } = useParams()

  if (!template) {
    throw new Error("No template found in the URL")
  }

  return template
}

export const TemplatePage: FC<React.PropsWithChildren<unknown>> = () => {
  const organizationId = useOrganizationId()
  const templateName = useTemplateName()
  const [templateState, templateSend] = useMachine(templateMachine, {
    context: {
      templateName,
      organizationId,
    },
  })

  const {
    template,
    activeTemplateVersion,
    templateResources,
    templateVersions,
    deleteTemplateError,
    templateDAUs,
  } = templateState.context
  const xServices = useContext(XServiceContext)
  const permissions = useSelector(xServices.authXService, selectPermissions)
  const isLoading =
    !template || !activeTemplateVersion || !templateResources || !permissions || !templateDAUs

  const handleDeleteTemplate = () => {
    templateSend("DELETE")
  }

  if (isLoading) {
    return <Loader />
  }

  if (templateState.matches("deleted")) {
    return <Navigate to="/templates" />
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${template.name} · Template`)}</title>
      </Helmet>
      <TemplatePageView
        template={template}
        activeTemplateVersion={activeTemplateVersion}
        templateResources={templateResources}
        templateVersions={templateVersions}
        templateDAUs={templateDAUs}
        canDeleteTemplate={permissions.deleteTemplates}
        handleDeleteTemplate={handleDeleteTemplate}
        deleteTemplateError={deleteTemplateError}
      />

      <DeleteDialog
        isOpen={templateState.matches("confirmingDelete")}
        confirmLoading={templateState.matches("deleting")}
        entity="template"
        name={template.name}
        onConfirm={() => {
          templateSend("CONFIRM_DELETE")
        }}
        onCancel={() => {
          templateSend("CANCEL_DELETE")
        }}
      />
    </>
  )
}

export default TemplatePage
