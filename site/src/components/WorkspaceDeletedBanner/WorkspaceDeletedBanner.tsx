import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import Alert from "@material-ui/lab/Alert"
import AlertTitle from "@material-ui/lab/AlertTitle"
import { FC } from "react"
import { useNavigate } from "react-router-dom"

const Language = {
  bannerTitle: "This workspace has been deleted and cannot be edited.",
  createWorkspaceCta: "Create new workspace",
}

export const WorkspaceDeletedBanner: FC = () => {
  const styles = useStyles()
  const navigate = useNavigate()

  return (
    <Alert
      className={styles.root}
      action={
        <Button color="inherit" onClick={() => navigate(`/workspaces/new`)} size="small">
          {Language.createWorkspaceCta}
        </Button>
      }
      severity="warning"
    >
      <AlertTitle>{Language.bannerTitle}</AlertTitle>
    </Alert>
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
