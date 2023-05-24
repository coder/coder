import Button from "@mui/material/Button"
import { FC } from "react"
import * as TypesGen from "api/typesGenerated"
import { Alert } from "components/Alert/Alert"
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
    <Button onClick={handleClick} size="small" variant="text">
      {t("ctas.createWorkspaceCta")}
    </Button>
  )

  return (
    <Maybe condition={workspace.latest_build.status === "deleted"}>
      <Alert severity="warning" actions={[NewWorkspaceButton]}>
        {t("warningsAndErrors.workspaceDeletedWarning")}
      </Alert>
    </Maybe>
  )
}
