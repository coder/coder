import Box from "@material-ui/core/Box"
import CircularProgress from "@material-ui/core/CircularProgress"
import React from "react"

export const Loader: React.FC<{ size?: number }> = ({ size = 26 }) => {
  return (
    <Box p={4}>
      <CircularProgress size={size} />
    </Box>
  )
}
