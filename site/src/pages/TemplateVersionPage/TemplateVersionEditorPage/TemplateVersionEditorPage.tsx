import { useMachine } from "@xstate/react"
import { TemplateEditor } from "components/TemplateVersionEditor/TemplateEditor"
import { TemplateVersionEditor } from "components/TemplateVersionEditor/TemplateVersionEditor"
import { useOrganizationId } from "hooks/useOrganizationId"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { useParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { templateVersionMachine } from "xServices/templateVersion/templateVersionXService"
import { templateVersionEditorMachine } from "xServices/templateVersionEditor/templateVersionEditorXService"

type Params = {
  version: string
  template: string
}

export const TemplateVersionEditorPage: FC = () => {
  const { version: versionName, template: templateName } = useParams() as Params
  const orgId = useOrganizationId()
  const [versionState] = useMachine(templateVersionMachine, {
    context: { templateName, versionName, orgId },
  })
  const [editorState, sendEvent] = useMachine(templateVersionEditorMachine, {
    context: { orgId },
  })
  const { t } = useTranslation("templateVersionPage")

  return (
    <>
      <Helmet>
        <title>
          {pageTitle(`${t("title")} ${versionName} · ${templateName}`)}
        </title>
      </Helmet>

      {versionState.context.template &&
        versionState.context.currentFiles &&
        versionState.context.currentVersion && (
          <TemplateVersionEditor
            template={versionState.context.template}
            templateVersion={
              editorState.context.version || versionState.context.currentVersion
            }
            initialFiles={versionState.context.currentFiles}
            onPreview={(files) => {
              if (!versionState.context.template) {
                throw new Error("no template")
              }
              sendEvent({
                type: "CREATE_BUILD",
                files: files,
                templateId: versionState.context.template.id,
              })
            }}
            onUpdate={() => {
              sendEvent({
                type: "UPDATE_ACTIVE",
              })
            }}
            disablePreview={editorState.hasTag("loading")}
            disableUpdate={
              editorState.hasTag("loading") ||
              editorState.context.version?.job.status !== "succeeded"
            }
            resources={editorState.context.resources}
            buildLogs={editorState.context.buildLogs}
          />
        )}
    </>
  )
}

export default TemplateVersionEditorPage
