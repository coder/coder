import Button from "@material-ui/core/Button"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { isWorkspaceDeleted } from "../../util/workspace"
import { WarningAlert } from "components/WarningAlert/WarningAlert"
import { useTranslation } from "react-i18next"
import { makeMockApiError } from "testHelpers/entities"

export interface WorkspaceDeletedBannerProps {
  workspace: TypesGen.Workspace
  handleClick: () => void
}

export const WorkspaceDeletedBanner: FC<React.PropsWithChildren<WorkspaceDeletedBannerProps>> = ({
  workspace,
  handleClick,
}) => {
  const { t } = useTranslation("workspacePage")

  if (!isWorkspaceDeleted(workspace)) {
    return null
  }

  const NewWorkspaceButton = (
    <Button onClick={handleClick} size="small">
      {t("ctas.createWorkspaceCta")}
    </Button>
  )

  // return (
  //   <WarningAlert
  //     text={t("warningsAndErrors.workspaceDeletedWarning")}
  //     actions={[NewWorkspaceButton]}
  //     severity="warning"
  //   />
  // )

  const error = makeMockApiError({
    message: "Email or password was invalid",
    detail: "you screwd up",
    validations: [
      {
        field: "password",
        detail: "Password is invalid.",
      },
    ],
  })

  return (
    <WarningAlert
      text={t("warningsAndErrors.workspaceDeletedWarning")}
      actions={[NewWorkspaceButton]}
      severity="error"
      error={error}
    />
  )
}
