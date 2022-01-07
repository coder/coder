import React from "react"
import { Box } from "@material-ui/core"

import { Confetti, Triangle } from "../components"

/**
 * CreateWorkspaceForm
 *
 * Placeholder component for the new v2 workspace creation flow
 */
export const CreateWorkspace: React.FC = () => {
  return (
    <Confetti
      style={{
        minHeight: "500px",
        display: "flex",
        flexDirection: "column",
        justifyContent: "center",
        alignItems: "center",
      }}
    >
      <Triangle />

      <Box m={"2em"}>NEXT STEP: Let's create a workspace with a v2 provisioner!</Box>
    </Confetti>
  )
}
