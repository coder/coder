import Box from "@mui/material/Box"
import { WorkspaceBuild } from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"

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
  const orphanCommand = `coder rm ${
    build.workspace_owner_name + "/" + build.workspace_name
  } --orphan`
  return (
    <AlertBanner
      severity="error"
      text={
        <Box>
          {Language.stateMessage} A template admin may run{" "}
          <Box
            component="code"
            display="inline-block"
            width="fit-content"
            fontWeight={600}
          >
            `{orphanCommand}`
          </Box>{" "}
          to delete the workspace skipping resource destruction.
        </Box>
      }
    />
  )
}
