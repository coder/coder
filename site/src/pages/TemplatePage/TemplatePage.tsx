import { useMachine, useSelector } from "@xstate/react"
import { ConfirmDialog } from "components/ConfirmDialog/ConfirmDialog"
import { FC, useContext } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
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
  const { t } = useTranslation("templatePage")
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

      <ConfirmDialog
        type="delete"
        hideCancel={false}
        open={templateState.matches("confirmingDelete")}
        confirmLoading={templateState.matches("deleting")}
        title={t("deleteDialog.title")}
        confirmText={t("deleteDialog.confirm")}
        onConfirm={() => {
          templateSend("CONFIRM_DELETE")
        }}
        onClose={() => {
          templateSend("CANCEL_DELETE")
        }}
        description={<>{t("deleteDialog.message")}</>}
      />
    </>
  )
}
