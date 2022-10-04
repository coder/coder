import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import Alert from "@material-ui/lab/Alert"
import AlertTitle from "@material-ui/lab/AlertTitle"
import { Maybe } from "components/Conditionals/Maybe"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"

const Language = {
  bannerTitle: "This workspace has been deleted and cannot be edited.",
  createWorkspaceCta: "Create new workspace",
}

export interface WorkspaceDeletedBannerProps {
  workspace: TypesGen.Workspace
  handleClick: () => void
}

export const WorkspaceDeletedBanner: FC<React.PropsWithChildren<WorkspaceDeletedBannerProps>> = ({
  workspace,
  handleClick,
}) => {
  const styles = useStyles()

  return (
    <Maybe condition={workspace.latest_build.status === "deleted"}>
      <Alert
        className={styles.root}
        action={
          <Button color="inherit" onClick={handleClick} size="small">
            {Language.createWorkspaceCta}
          </Button>
        }
        severity="warning"
      >
        <AlertTitle>{Language.bannerTitle}</AlertTitle>
      </Alert>
    </Maybe>
  )
}

export const useStyles = makeStyles(() => {
  return {
    root: {
      alignItems: "center",
      "& .MuiAlertTitle-root": {
        marginBottom: "0px",
      },
    },
  }
})
