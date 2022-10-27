import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { BaseIcon } from "./BaseIcon"
import { ShareIcon } from "./ShareIcon"

export interface AppPreviewProps {
  app: TypesGen.WorkspaceApp
}

export const AppPreviewLink: FC<AppPreviewProps> = ({ app }) => {
  const styles = useStyles()

  return (
    <Button
      size="small"
      startIcon={<BaseIcon app={app} />}
      endIcon={<ShareIcon app={app} />}
      className={styles.button}
    >
      <span className={styles.appName}>{app.name}</span>
    </Button>
  )
}

const useStyles = makeStyles((theme) => ({
  button: {
    whiteSpace: "nowrap",
    cursor: "default",
  },

  appName: {
    marginRight: theme.spacing(1),
  },
}))
