import { makeStyles } from "@mui/styles"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import * as TypesGen from "api/typesGenerated"
import { BaseIcon } from "./BaseIcon"
import { ShareIcon } from "./ShareIcon"

interface AppPreviewProps {
  app: TypesGen.WorkspaceApp
}

export const AppPreviewLink: FC<AppPreviewProps> = ({ app }) => {
  const styles = useStyles()

  return (
    <Stack
      className={styles.appPreviewLink}
      alignItems="center"
      direction="row"
      spacing={1}
    >
      <BaseIcon app={app} />
      {app.display_name}
      <ShareIcon app={app} />
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  appPreviewLink: {
    padding: theme.spacing(0.25, 1.5),
    borderRadius: 9999,
    border: `1px solid ${theme.palette.divider}`,
    color: theme.palette.text.primary,
    background: theme.palette.background.paper,
    flexShrink: 0,
    width: "fit-content",
    fontSize: 12,

    "& img, & svg": {
      width: 13,
    },
  },
}))
