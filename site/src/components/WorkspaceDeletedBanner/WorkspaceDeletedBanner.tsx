import Button from "@material-ui/core/Button"
import { FC } from "react"
import * as TypesGen from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { useTranslation } from "react-i18next"
import { Maybe } from "components/Conditionals/Maybe"

export interface WorkspaceDeletedBannerProps {
  workspace: TypesGen.Workspace
  handleClick: () => void
}

export const WorkspaceDeletedBanner: FC<
  React.PropsWithChildren<WorkspaceDeletedBannerProps>
> = ({ workspace, handleClick }) => {
  const { t } = useTranslation("workspacePage")

  const NewWorkspaceButton = (
    <Button onClick={handleClick} size="small">
      {t("ctas.createWorkspaceCta")}
    </Button>
  )

  return (
    <Maybe condition={workspace.latest_build.status === "deleted"}>
      <AlertBanner
        text={t("warningsAndErrors.workspaceDeletedWarning")}
        actions={[NewWorkspaceButton]}
        severity="warning"
      />
    </Maybe>
  )
}
