import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"

export interface WorkspaceAppErrorPageViewProps {
  appName: string
  message: string
}

export const WorkspaceAppErrorPageView: FC<
  React.PropsWithChildren<WorkspaceAppErrorPageViewProps>
> = (props) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <h1 className={styles.title}>{props.appName} is offline!</h1>
      <p className={styles.message}>{props.message}</p>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    flex: 1,
    padding: theme.spacing(10),
  },
  title: {
    textAlign: "center",
  },
  message: {
    textAlign: "center",
  },
}))
