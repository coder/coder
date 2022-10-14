import { makeStyles } from "@material-ui/core/styles"
import { WorkspaceBuild } from "api/typesGenerated"
import { CodeExample } from "components/CodeExample/CodeExample"
import { Stack } from "components/Stack/Stack"

const Language = {
  stateMessage:
    "The workspace may have failed to delete due to a Terraform state mismatch.",
}

export interface WorkspaceBuildStateErrorProps {
  build: WorkspaceBuild
}

export const WorkspaceBuildStateError: React.FC<
  WorkspaceBuildStateErrorProps
> = ({ build }) => {
  const styles = useStyles()

  const orphanCommand = `coder rm ${
    build.workspace_owner_name + "/" + build.workspace_name
  } --orphan`
  return (
    <Stack className={styles.root}>
      <Stack direction="row" alignItems="center" className={styles.messageBox}>
        <Stack direction="row" spacing={0}>
          <span className={styles.errorMessage}>
            {Language.stateMessage} A template admin may run{" "}
            <CodeExample inline code={orphanCommand} /> to delete the workspace
            skipping resource destruction.
          </span>
        </Stack>
      </Stack>
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    background: theme.palette.warning.main,
    padding: `${theme.spacing(2)}px`,
    borderRadius: theme.shape.borderRadius,
    gap: 0,
  },
  flex: {
    display: "flex",
  },
  messageBox: {
    justifyContent: "space-between",
  },
  errorMessage: {
    marginRight: `${theme.spacing(1)}px`,
  },
  iconButton: {
    padding: 0,
  },
}))
