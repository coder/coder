import React from "react"
import { Box } from "@material-ui/core"

/**
 * CreateWorkspaceForm
 *
 * Placeholder component for the new v2 workspace creation flow
 */
export const CreateWorkspace: React.FC = () => {
  return (
    <div
      style={{
        minHeight: "500px",
        display: "flex",
        flexDirection: "column",
        justifyContent: "center",
        alignItems: "center",
      }}
    >
      <Box m={"2em"}>NEXT STEP: Let's create a workspace with a v2 provisioner!</Box>
    </div>
  )
}
