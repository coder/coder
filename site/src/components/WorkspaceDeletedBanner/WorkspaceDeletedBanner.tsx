import Button from "@material-ui/core/Button"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { isWorkspaceDeleted } from "../../util/workspace"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { useTranslation } from "react-i18next"

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

  return (
    <AlertBanner
      text={t("warningsAndErrors.workspaceDeletedWarning")}
      actions={[NewWorkspaceButton]}
      severity="warning"
    />
  )
}
