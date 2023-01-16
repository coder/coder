import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Maybe } from "components/Conditionals/Maybe"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { Loader } from "components/Loader/Loader"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import { useNavigate } from "react-router-dom"
import { ChangeWorkspaceVersionContext } from "xServices/workspace/changeWorkspaceVersionXService"
import { WorkspaceChangeVersionForm } from "./WorkspaceChangeVersionForm"

export interface WorkspaceChangeVersionPageViewProps {
  isUpdating: boolean
  context: ChangeWorkspaceVersionContext
  onSubmit: (versionId: string) => void
}

export const WorkspaceChangeVersionPageView: FC<
  WorkspaceChangeVersionPageViewProps
> = ({ context, onSubmit, isUpdating }) => {
  const navigate = useNavigate()
  const { workspace, templateVersions, template, error } = context

  return (
    <FullPageForm title="Change version">
      <Stack>
        <Maybe condition={Boolean(error)}>
          <AlertBanner severity="error" error={error} />
        </Maybe>
        {workspace && template && templateVersions ? (
          <WorkspaceChangeVersionForm
            isLoading={isUpdating}
            versions={templateVersions}
            workspace={workspace}
            template={template}
            onSubmit={onSubmit}
            onCancel={() => {
              navigate(-1)
            }}
          />
        ) : (
          <Loader />
        )}
      </Stack>
    </FullPageForm>
  )
}
