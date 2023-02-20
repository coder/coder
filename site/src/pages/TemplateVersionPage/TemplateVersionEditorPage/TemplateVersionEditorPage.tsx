import { useMachine } from "@xstate/react"
import { TemplateVersionEditor } from "components/TemplateVersionEditor/TemplateVersionEditor"
import { useOrganizationId } from "hooks/useOrganizationId"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { templateVersionEditorMachine } from "xServices/templateVersionEditor/templateVersionEditorXService"
import { useTemplateVersionData } from "./data"

type Params = {
  version: string
  template: string
}

export const TemplateVersionEditorPage: FC = () => {
  const { version: versionName, template: templateName } = useParams() as Params
  const orgId = useOrganizationId()
  const [editorState, sendEvent] = useMachine(templateVersionEditorMachine, {
    context: { orgId },
  })
  const { isSuccess, data } = useTemplateVersionData(
    {
      orgId,
      templateName,
      versionName,
    },
    {
      onSuccess(data) {
        sendEvent({ type: "INITIALIZE", tarReader: data.tarReader })
      },
    },
  )

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${templateName} · Template Editor`)}</title>
      </Helmet>

      {isSuccess && (
        <TemplateVersionEditor
          template={data.template}
          templateVersion={editorState.context.version || data.version}
          defaultFileTree={data.fileTree}
          onPreview={(fileTree) => {
            sendEvent({
              type: "CREATE_VERSION",
              fileTree,
              templateId: data.template.id,
            })
          }}
          onUpdate={() => {
            sendEvent({
              type: "UPDATE_ACTIVE_VERSION",
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
